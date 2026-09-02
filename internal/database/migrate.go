package database

import (
	"zawaj/internal/models"

	"gorm.io/gorm"
)

// Migrate runs GORM AutoMigrate for all models. Used only by tests for a fast,
// self-contained schema. Production uses RunMigrations (versioned SQL, ADR-002).
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Wedding{},
		&models.Membership{},
		&models.InviteLink{},
		&models.WeddingInvite{},
		&models.Guest{},
		&models.GuestNote{},
		&models.ActivityLog{},
		&models.Notification{},
		&models.DeviceToken{},
		&models.RefreshToken{},
		&models.IdempotencyKey{},
		&models.AuditLog{},
	)
}
