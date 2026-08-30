package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Upload records a single file import for an account: the file it came from
// (created_at is the upload time) and the date range of the transactions it
// contained.
type Upload struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	FileName string `gorm:"not null" json:"file_name"`

	// StartDate and EndDate bound the dates of the transactions in the file, so
	// we know the range covered. Nil until the file is parsed.
	StartDate *time.Time `gorm:"type:date" json:"start_date"`
	EndDate   *time.Time `gorm:"type:date" json:"end_date"`

	AccountID uuid.UUID `gorm:"type:uuid" json:"account_id"`
} // @name Upload
