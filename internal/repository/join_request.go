package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// JoinRequestRepo persists requests to join a wedding through an invite link.
type JoinRequestRepo struct{ db *gorm.DB }

// NewJoinRequestRepo builds the repo.
func NewJoinRequestRepo(db *gorm.DB) *JoinRequestRepo { return &JoinRequestRepo{db: db} }

// JoinRequestView is a pending request enriched with the requester's identity,
// for the owner's review list.
type JoinRequestView struct {
	ID          uuid.UUID   `json:"id"`
	UserID      uuid.UUID   `json:"user_id"`
	Username    string      `json:"username"`
	DisplayName string      `json:"display_name"`
	Role        models.Role `json:"role"`
	CreatedAt   time.Time   `json:"created_at"`
}

// ErrJoinQueueFull: the wedding already has the maximum number of pending
// requests.
var ErrJoinQueueFull = errors.New("join request queue full")

// PendingResult is the outcome of CreatePending.
type PendingResult struct {
	Request *models.JoinRequest
	// Created: this call inserted the request (false: it was already pending).
	Created bool
	// FirstInQueue: the wedding had no other pending request, so the owner
	// isn't already aware of one (used to batch owner pushes).
	FirstInQueue bool
}

// CreatePending records a pending request, or returns the one already pending
// for (wedding, user) — repeated taps never create duplicates. At most
// maxPending requests may be pending per wedding (ErrJoinQueueFull beyond
// that, unless the user already has one). Requests for a wedding are
// serialized by a transaction-scoped advisory lock so the cap holds under
// concurrency; the partial unique index idx_join_requests_pending backs the
// one-per-user rule.
func (r *JoinRequestRepo) CreatePending(ctx context.Context, weddingID, userID, linkID uuid.UUID, role models.Role, maxPending int) (*PendingResult, error) {
	var out PendingResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "join_requests:"+weddingID.String()).Error; err != nil {
			return err
		}
		var existing models.JoinRequest
		err := tx.Where("wedding_id = ? AND user_id = ? AND status = ?", weddingID, userID, models.JoinPending).First(&existing).Error
		if err == nil {
			out = PendingResult{Request: &existing}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var pending int64
		if err := pendingForWedding(tx, weddingID).Count(&pending).Error; err != nil {
			return err
		}
		if pending >= int64(maxPending) {
			return ErrJoinQueueFull
		}
		fresh := &models.JoinRequest{WeddingID: weddingID, UserID: userID, LinkID: linkID, Role: role, Status: models.JoinPending}
		res := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "wedding_id"}, {Name: "user_id"}},
			// Literal predicate, never a bind parameter: under a cached generic
			// plan Postgres can't prove "status = $n" implies the partial index's
			// "status = 'pending'" and rejects the ON CONFLICT target (42P10).
			TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "status = 'pending'"}}},
			DoNothing:   true,
		}).Create(fresh)
		if res.Error != nil {
			return res.Error
		}
		out = PendingResult{Request: fresh, Created: res.RowsAffected == 1, FirstInQueue: pending == 0}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// pendingForWedding scopes to a wedding's pending requests from non-members
// (a member's leftover request has nothing to decide).
func pendingForWedding(tx *gorm.DB, weddingID uuid.UUID) *gorm.DB {
	return tx.Model(&models.JoinRequest{}).
		Where("join_requests.wedding_id = ? AND join_requests.status = ?", weddingID, models.JoinPending).
		Where("NOT EXISTS (SELECT 1 FROM memberships m WHERE m.wedding_id = join_requests.wedding_id AND m.user_id = join_requests.user_id)")
}

// PendingByID returns a pending request of the wedding, or ErrNotFound.
func (r *JoinRequestRepo) PendingByID(ctx context.Context, weddingID, requestID uuid.UUID) (*models.JoinRequest, error) {
	var req models.JoinRequest
	err := r.db.WithContext(ctx).
		Where("id = ? AND wedding_id = ? AND status = ?", requestID, weddingID, models.JoinPending).
		First(&req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// LastDeclinedAt returns when the user's latest declined request for the
// wedding was decided, or ErrNotFound if none.
func (r *JoinRequestRepo) LastDeclinedAt(ctx context.Context, weddingID, userID uuid.UUID) (time.Time, error) {
	var at sql.NullTime
	err := r.db.WithContext(ctx).Model(&models.JoinRequest{}).
		Select("MAX(decided_at)").
		Where("wedding_id = ? AND user_id = ? AND status = ?", weddingID, userID, models.JoinDeclined).
		Row().Scan(&at)
	if err != nil {
		return time.Time{}, err
	}
	if !at.Valid {
		return time.Time{}, ErrNotFound
	}
	return at.Time, nil
}

// ListPending returns a wedding's pending requests, oldest first, at most limit.
func (r *JoinRequestRepo) ListPending(ctx context.Context, weddingID uuid.UUID, limit int) ([]JoinRequestView, error) {
	out := []JoinRequestView{}
	err := r.db.WithContext(ctx).
		Table("join_requests AS j").
		Select("j.id, j.user_id, u.username, u.display_name, j.role, j.created_at").
		Joins("JOIN users u ON u.id = j.user_id").
		Where("j.wedding_id = ? AND j.status = ?", weddingID, models.JoinPending).
		// Defensive: a member's leftover request has nothing to decide.
		Where("NOT EXISTS (SELECT 1 FROM memberships m WHERE m.wedding_id = j.wedding_id AND m.user_id = j.user_id)").
		Order("j.created_at ASC").
		Limit(limit).
		Scan(&out).Error
	return out, err
}

// ApproveOutcome is what approving a request did.
type ApproveOutcome int

const (
	// Approved: a membership was created at the approved role.
	Approved ApproveOutcome = iota
	// AlreadyMember: they joined some other way meanwhile; their membership is
	// untouched and the request is closed.
	AlreadyMember
	// Removed: the owner removed them after they asked (ceiling "none"); no
	// membership is created and the request is closed.
	Removed
)

// Approve decides a pending request in one transaction: it locks the request
// row, re-checks the owner's last decision about the user, creates the
// membership at role (recording the link it came through), records role as
// the member's ceiling (an owner decision), and marks the request approved.
// ErrNotFound if the request isn't pending in this wedding. See
// ApproveOutcome for the other cases; in those the request is closed (and
// committed) rather than approved.
func (r *JoinRequestRepo) Approve(ctx context.Context, weddingID, requestID, deciderID uuid.UUID, role models.Role) (*models.JoinRequest, ApproveOutcome, error) {
	var req models.JoinRequest
	outcome := Approved
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPending(tx, weddingID, requestID, &req); err != nil {
			return err
		}
		var ceiling string
		if err := tx.Raw("SELECT max_role FROM membership_ceilings WHERE wedding_id = ? AND user_id = ?",
			weddingID, req.UserID).Scan(&ceiling).Error; err != nil {
			return err
		}
		if ceiling == CeilingRemoved {
			outcome = Removed
			return decide(tx, &req, models.JoinClosed, deciderID)
		}
		linkID := req.LinkID
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.Membership{
			WeddingID: weddingID, UserID: req.UserID, Role: role, JoinedAt: time.Now(), ViaLinkID: &linkID,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			outcome = AlreadyMember
			return decide(tx, &req, models.JoinClosed, deciderID)
		}
		if err := setCeiling(tx, weddingID, req.UserID, string(role)); err != nil {
			return err
		}
		req.Role = role
		return decide(tx, &req, models.JoinApproved, deciderID)
	})
	if err != nil {
		return nil, Approved, err
	}
	return &req, outcome, nil
}

// Decline marks a pending request declined. ErrNotFound if it isn't pending in
// this wedding.
func (r *JoinRequestRepo) Decline(ctx context.Context, weddingID, requestID, deciderID uuid.UUID) (*models.JoinRequest, error) {
	var req models.JoinRequest
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPending(tx, weddingID, requestID, &req); err != nil {
			return err
		}
		return decide(tx, &req, models.JoinDeclined, deciderID)
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

// lockPending loads the pending request FOR UPDATE so concurrent decisions on
// it serialize (the second one then sees it decided → ErrNotFound).
func lockPending(tx *gorm.DB, weddingID, requestID uuid.UUID, req *models.JoinRequest) error {
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND wedding_id = ? AND status = ?", requestID, weddingID, models.JoinPending).
		First(req).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func decide(tx *gorm.DB, req *models.JoinRequest, status models.JoinRequestStatus, deciderID uuid.UUID) error {
	now := time.Now()
	req.Status, req.DecidedAt, req.DecidedBy = status, &now, &deciderID
	return tx.Model(&models.JoinRequest{}).Where("id = ?", req.ID).Updates(map[string]any{
		"status": status, "role": req.Role, "decided_at": now, "decided_by": deciderID,
	}).Error
}
