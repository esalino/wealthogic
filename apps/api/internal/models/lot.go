package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Lot struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	HoldingID uuid.UUID `gorm:"type:uuid;not null" json:"holding_id"`

	PurchaseDate     time.Time `gorm:"not null"  json:"purchase_date"`
	PurchaseQuantity float64   `gorm:"not null"  json:"purchase_quantity"`
	PurchasePrice    float64   `gorm:"not null"  json:"purchase_price"`
	RemainingQuantity float64  `json:"remaining_quantity"`
	RealizedGains    float64   `json:"realized_gains"`
}
