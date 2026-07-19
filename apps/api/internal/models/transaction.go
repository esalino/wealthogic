package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Transaction struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	PositionID uuid.UUID `gorm:"type:uuid" json:"position_id"`

	AssetType        string `gorm:"not null"  json:"asset_type"`
	Symbol           string `json:"symbol"`
	AssetDescription string `gorm:"not null"  json:"asset_description"`

	TransactionType        string    `gorm:"not null"  json:"transaction_type"`
	TransactionDescription string    `gorm:"not null"  json:"transaction_description"`
	TransactionDate        time.Time `gorm:"not null"  json:"transaction_date"`
	TransactionQuantity    float64   `gorm:"not null"  json:"transaction_quantity"`
	TransactionPrice       float64   `gorm:"not null"  json:"transaction_price"`

	RealizedGains float64 `json:"realized_gains"`
}
