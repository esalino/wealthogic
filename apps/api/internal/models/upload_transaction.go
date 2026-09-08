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

	// A row links to exactly one of the domain records the import produced: a
	// Transaction (trades) or a Distribution (dividends/interest). Both are
	// nullable so either kind can be absent; a transaction still has 0 or 1
	// upload_transaction (the unique index treats NULLs as distinct, so many
	// distribution rows can share a NULL transaction_id).
	TransactionID  *uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"transaction_id"`
	DistributionID *uuid.UUID `gorm:"type:uuid;index"       json:"distribution_id"`

	// UploadID ties this row to the file import it came from.
	UploadID uuid.UUID `gorm:"type:uuid" json:"upload_id"`
} // @name UploadTransaction
