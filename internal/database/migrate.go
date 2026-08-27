package database

import (
	"zawaj/internal/models"

	"gorm.io/gorm"
)

// Migrate runs AutoMigrate for all models. Each phase appends its models here.
// NOTE: AutoMigrate is for dev only — replace with versioned migrations (P6)
// before any production data exists (see ADR-002).
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Wedding{},
		&models.Membership{},
		&models.InviteLink{},
		&models.Guest{},
		&models.GuestNote{},
		&models.ActivityLog{},
		&models.Notification{},
	)
}
