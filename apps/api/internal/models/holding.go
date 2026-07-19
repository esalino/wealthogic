package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Holding represents a single holding. A holding can be made up of multiple child positions.
type Holding struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	AssetType   string `gorm:"not null"  json:"asset_type"`
	Symbol      string `json:"symbol"`
	Description string `gorm:"not null"  json:"description"`

	LastPrice float64 `json:"last_price"`

	Positions []Position `json:"positions,omitempty"`
}
