package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UploadTransaction is a transaction that belongs to an Upload. It's a copy of
// Transaction for now, plus the UploadID linking it to the file it came from.
type UploadTransaction struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	AssetType        *string `json:"asset_type"`
	Symbol           string  `json:"symbol"`
	AssetDescription *string `json:"asset_description"`

	Action   string    `gorm:"not null"        json:"action"`
	Date     time.Time `gorm:"not null;type:date" json:"date"`
	Quantity *float64  `json:"quantity"`
	Price    *float64  `json:"price"`
	Amount   float64   `gorm:"not null"        json:"amount"`

	Commission float64 `json:"commission"`
	Fees       float64 `json:"fees"`

	SettlementDate *time.Time `gorm:"type:date" json:"settlement_date"`

	RealizedGains float64 `json:"realized_gains"`

	// TransactionID links this to its Transaction. A transaction has 0 or 1
	// upload_transaction (enforced by the unique index), and the holding and
	// account can be reached through the transaction.
	TransactionID uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"transaction_id"`

	// UploadID ties this row to the file import it came from.
	UploadID uuid.UUID `gorm:"type:uuid" json:"upload_id"`
} // @name UploadTransaction
