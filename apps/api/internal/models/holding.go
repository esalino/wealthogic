package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Holding represents a single holding. A holding can be made up of multiple child tax lots.
type Holding struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	AssetType   string `gorm:"not null"  json:"asset_type"`
	Symbol      string `json:"symbol"`
	Description string `gorm:"not null"  json:"description"`

	LastPrice float64 `json:"last_price"`

	Quantity     float64 `gorm:"not null;default:0"  json:"purchase_quantity"`
	CurrentValue float64 `json:"current_value"`

	AverageCostBasis float64 `json:"average_cost_basis"`
	CostBasisTotal   float64 `gorm:"not null;default:0" json:"cost_basis_total"`

	GainUnrealizedPercent float64 `gorm:"not null;default:0" json:"gain_unrealized_percent"`
	GainUnrealizedAmount  float64 `gorm:"not null;default:0" json:"gain_unrealized_amount"`

	GainRealizedPercent float64 `json:"gain_realized_percent"`
	GainRealizedAmount  float64 `json:"gain_realized_amount"`
	DividendIncome      float64 `json:"dividend_income"`

	TaxLots []TaxLot `json:"tax_lots,omitempty"`
}
