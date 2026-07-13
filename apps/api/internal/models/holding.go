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

	AccountID      uuid.UUID `gorm:"type:uuid" json:"account_id"`
	AccountName    string    `gorm:"not null"  json:"account_name"`
	InvestmentType string    `gorm:"not null"  json:"investment_type"`
	Symbol         string    `json:"symbol"`
	Description    string    `gorm:"not null"  json:"description"`

	Quantity float64 `json:"quantity"`

	LastPrice float64 `json:"last_price"`

	CurrentValue float64 `json:"current_value"`

	TotalGainLossDollar  float64 `json:"total_gain_loss_dollar"`
	TotalGainLossPercent float64 `json:"total_gain_loss_percent"`

	CostBasisTotal   float64 `json:"cost_basis_total"`
	CostBasisAverage float64 `json:"cost_basis_average"`

	Lots []Lot `json:"lots,omitempty"`
}
