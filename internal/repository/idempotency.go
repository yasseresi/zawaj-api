package repository

import (
	"context"
	"errors"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// IdempotencyRepo stores and retrieves idempotent request outcomes.
type IdempotencyRepo struct{ db *gorm.DB }

// NewIdempotencyRepo builds the repo.
func NewIdempotencyRepo(db *gorm.DB) *IdempotencyRepo { return &IdempotencyRepo{db: db} }

// Get returns the stored record for (userID, key), or ErrNotFound.
func (r *IdempotencyRepo) Get(ctx context.Context, userID uuid.UUID, key string) (*models.IdempotencyKey, error) {
	var row models.IdempotencyKey
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND key = ?", userID, key).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Put stores a request outcome. Callers check Get first; on a rare concurrent
// duplicate the unique (user_id, key) index rejects the second insert and the
// caller ignores that error, so the first writer wins and the response is stable.
func (r *IdempotencyRepo) Put(ctx context.Context, userID uuid.UUID, key, requestHash string, status int, body []byte) error {
	row := models.IdempotencyKey{
		Base:        models.Base{ID: uuid.New()},
		UserID:      userID,
		Key:         key,
		RequestHash: requestHash,
		StatusCode:  status,
		Response:    body,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}
