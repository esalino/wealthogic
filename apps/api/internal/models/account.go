package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Account struct {
	ID          uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"                                           json:"-"`
	AccountName string         `gorm:"not null"                                        json:"account_name"`
	AccountType string         `gorm:"not null"                                        json:"account_type"`
	Balance     float64        `json:"balance"`
	Assets      []Asset        `json:"assets,omitempty"`
}
