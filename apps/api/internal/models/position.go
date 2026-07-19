package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Position struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	AssetType   string `gorm:"not null"  json:"asset_type"`
	Symbol      string `json:"symbol"`
	Description string `gorm:"not null"  json:"description"`

	PurchaseDate      time.Time `gorm:"not null"  json:"purchase_date"`
	PurchaseQuantity  float64   `gorm:"not null"  json:"purchase_quantity"`
	PurchasePrice     float64   `gorm:"not null"  json:"purchase_price"`
	RemainingQuantity float64   `json:"remaining_quantity"`

	HoldingID    uuid.UUID     `gorm:"type:uuid" json:"holding_id"`
	Transactions []Transaction `json:"transactions,omitempty"`
}
