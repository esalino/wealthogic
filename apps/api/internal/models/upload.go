package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Upload records a single file import for an account: the file it came from
// (created_at is the upload time) and the date range of the transactions it
// contained.
//
// Every import gets one, whatever it produced. A positions snapshot creates no
// transaction rows, so without its own Upload row it would leave no trace in
// history at all - the file would look like it had never been uploaded.
type Upload struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	// FileName is the bare name of the file, with any directory part stripped:
	// history reads better as "Positions.csv" than as the path a client
	// happened to send.
	FileName string `gorm:"not null" json:"file_name"`

	// FileType is the kind of data the file carried - holdings or transactions -
	// which is what distinguishes two imports of the same account on the same
	// day from each other. Empty on rows imported before it was recorded.
	FileType string `json:"file_type"`

	// StartDate and EndDate bound the dates of the transactions in the file, so
	// we know the range covered. Both are nil for a positions snapshot, which
	// describes one moment rather than a span.
	StartDate *time.Time `gorm:"type:date" json:"start_date"`
	EndDate   *time.Time `gorm:"type:date" json:"end_date"`

	AccountID uuid.UUID `gorm:"type:uuid" json:"account_id"`
} // @name Upload
