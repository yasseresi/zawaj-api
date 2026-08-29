package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// Meta builds a jsonb audit-meta value from a map. Returns nil for a nil map.
func Meta(m map[string]any) models.JSON {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

// AuditService records security/compliance events. Recording is best-effort: a
// failure is logged but never fails the originating request.
type AuditService struct {
	repo *repository.AuditRepo
	log  *slog.Logger
}

// NewAuditService builds the service.
func NewAuditService(repo *repository.AuditRepo, log *slog.Logger) *AuditService {
	return &AuditService{repo: repo, log: log}
}

// Record writes an audit entry. actor and target may be nil for actorless or
// non-targeted events. meta carries extra structured detail (may be nil).
func (s *AuditService) Record(ctx context.Context, actor *uuid.UUID, action, targetType string, target *uuid.UUID, ip string, meta models.JSON) {
	e := &models.AuditLog{
		ActorID:    actor,
		Action:     action,
		TargetType: targetType,
		TargetID:   target,
		IP:         ip,
		Meta:       meta,
	}
	if err := s.repo.Insert(ctx, e); err != nil {
		s.log.Error("audit: record failed", "error", err, "action", action)
	}
}
