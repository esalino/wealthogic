package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Asset represents a single holding. An asset can be made of of multiple child positions.
type Asset struct {
	ID        uuid.UUID `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`

	AccountID   uuid.UUID `gorm:"type:uuid"`
	AccountName    string `gorm:"not null"`
	InvestmentType string `gorm:"not null"`
	Symbol         string
	Description    string `gorm:"not null"`

	Quantity float64

	LastPrice float64

	CurrentValue float64

	TotalGainLossDollar  float64
	TotalGainLossPercent float64

	PercentOfAccount float64

	CostBasisTotal   float64
	AverageCostBasis float64
}
