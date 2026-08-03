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

	AccountID uuid.UUID `gorm:"type:uuid" json:"account_id"`
} // @name Transaction
