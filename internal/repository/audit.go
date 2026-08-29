package repository

import (
	"context"

	"zawaj/internal/models"

	"gorm.io/gorm"
)

// AuditRepo persists security/compliance audit entries.
type AuditRepo struct{ db *gorm.DB }

// NewAuditRepo builds the repo.
func NewAuditRepo(db *gorm.DB) *AuditRepo { return &AuditRepo{db: db} }

// Insert writes one audit entry.
func (r *AuditRepo) Insert(ctx context.Context, e *models.AuditLog) error {
	return r.db.WithContext(ctx).Create(e).Error
}
