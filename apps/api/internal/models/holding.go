package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Holding status values. A holding is Open while any shares remain and Closed
// once the position has been fully sold off.
const (
	HoldingStatusOpen   = "Open"
	HoldingStatusClosed = "Closed"
)

// Asset types the tax rules single out. Kept here so the rules and the
// importer's classification agree on the spelling. Treasury income is exempt
// from state tax by default; money-market and treasury income is always ordinary
// (never a qualifiable dividend).
const (
	AssetTypeTreasury    = "Treasury"
	AssetTypeMoneyMarket = "Money Market"
)

// defaultStateExempt is the state-tax treatment implied by an asset type alone:
// U.S. Treasury income is exempt from state tax; everything else is taxable.
func defaultStateExempt(assetType string) bool {
	return assetType == AssetTypeTreasury
}

// Holding represents a single holding. A holding can be made up of multiple child tax lots.
type Holding struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	AssetType   string `gorm:"not null"  json:"asset_type"`
	Symbol      string `json:"symbol"`
	Description string `gorm:"not null"  json:"description"`

	Status string `gorm:"not null;default:Open" json:"status"`

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

	// StateTaxExempt overrides the state-tax treatment for this holding. Nil
	// means "derive from the asset type" (see ResolveStateExempt); a non-nil
	// value is an explicit user choice - e.g. a treasury-only ETF like TLT
	// flagged exempt even though ETFs aren't exempt by default.
	StateTaxExempt *bool `json:"state_tax_exempt"`

	// StateExempt is the resolved treatment (override, else the asset-type
	// default), computed for responses via AfterFind and never persisted.
	StateExempt bool `gorm:"-" json:"state_exempt"`
} // @name Holding

// ResolveStateExempt reports whether this holding's income is exempt from state
// tax: the per-holding override when set, otherwise the asset-type default.
func (h Holding) ResolveStateExempt() bool {
	if h.StateTaxExempt != nil {
		return *h.StateTaxExempt
	}
	return defaultStateExempt(h.AssetType)
}

// AfterFind populates the computed StateExempt field whenever a holding is read.
func (h *Holding) AfterFind(*gorm.DB) error {
	h.StateExempt = h.ResolveStateExempt()
	return nil
}
