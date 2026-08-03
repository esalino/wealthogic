package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaxLot represents a single purchase lot - a specific date, quantity, and
// price - within a Holding. Not limited to equities; any asset bought in
// discrete batches (shares, crypto, etc.) uses this to track cost basis.
type TaxLot struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	AssetType        string `gorm:"not null"  json:"asset_type"`
	Symbol           string `json:"symbol"`
	AssetDescription string `gorm:"not null"  json:"asset_description"`

	PurchaseDate      time.Time `gorm:"not null"  json:"purchase_date"`
	PurchaseQuantity  float64   `gorm:"not null"  json:"purchase_quantity"`
	PurchasePrice     float64   `gorm:"not null"  json:"purchase_price"`
	RemainingQuantity float64   `json:"remaining_quantity"`

	// HoldingID is nil until a matching Holding exists to link to - imported
	// transactions and holdings come from separate Fidelity exports and may
	// not arrive in the same order.
	HoldingID *uuid.UUID `gorm:"type:uuid" json:"holding_id"`
	AccountID uuid.UUID  `gorm:"type:uuid" json:"account_id"`
} // @name TaxLot
