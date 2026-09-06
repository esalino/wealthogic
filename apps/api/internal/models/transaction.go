package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Transaction represents a single account transaction.
type Transaction struct {
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

	// RemainingQuantity is set only on a stock buy, which doubles as a tax lot:
	// it's the shares of this purchase still open (quantity minus what later
	// sells have disposed). Nil for sells and other actions. Maintained as a
	// cache by the recompute; the Gain ledger is the source of truth.
	RemainingQuantity *float64 `json:"remaining_quantity"`

	// HoldingID is nil until a matching Holding exists to link to - imported
	// transactions and holdings come from separate Fidelity exports and may
	// not arrive in the same order.
	HoldingID *uuid.UUID `gorm:"type:uuid" json:"holding_id"`
	AccountID uuid.UUID  `gorm:"type:uuid" json:"account_id"`
} // @name Transaction
