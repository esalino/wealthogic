package models

import (
	"time"

	"github.com/google/uuid"
)

// Gain is one realized taxable event drawn from a tax lot: one row per
// (buy lot x sell) match - the granularity tax reporting needs, since a single
// sale can span a short-term and a long-term lot. It's a derived ledger, rebuilt
// when sells change, so no soft delete.
//
// Not every disposal realizes a capital gain. Disposing of a discount debt
// instrument realizes accreted discount, which is interest - so Category records
// what the event actually is (see DisposalIncomeType), and the lot fields stay
// just as meaningful either way: proceeds minus basis IS the accreted discount.
type Gain struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Category is the kind of income realized: IncomeTypeCapitalGain or
	// IncomeTypeInterest. Derived from the disposed asset's tax class, so it
	// stays correct when that class is corrected.
	Category string `gorm:"not null;default:capital_gain" json:"category"`

	HoldingID *uuid.UUID `gorm:"type:uuid;index" json:"holding_id"`
	AccountID uuid.UUID  `gorm:"type:uuid;index" json:"account_id"`
	Symbol    string     `json:"symbol"`
	AssetType string     `json:"asset_type"`

	// TransactionID is the realizing transaction (the sell). LotTransactionID is
	// the buy that supplied the disposed shares (capital gains only).
	TransactionID    uuid.UUID  `gorm:"type:uuid;index" json:"transaction_id"`
	LotTransactionID *uuid.UUID `gorm:"type:uuid;index" json:"lot_transaction_id"`

	AcquiredDate time.Time `gorm:"type:date" json:"acquired_date"`
	RealizedDate time.Time `gorm:"type:date" json:"realized_date"`

	Quantity  float64 `json:"quantity"`   // shares disposed from the lot
	CostBasis float64 `json:"cost_basis"` // includes the lot's commission and fees
	Proceeds  float64 `json:"proceeds"`   // net of the sell's commission and fees
	Term      string  `json:"term"`       // "short" | "long"

	Amount float64 `json:"amount"` // realized gain/loss = proceeds - cost_basis

	// Treatments is how each jurisdiction taxes this gain, recorded when it was
	// realized. The link is by (source_type, source_id) so one treatment table
	// can serve both this ledger and Distribution, which means it isn't a GORM
	// association - handlers that need it load it explicitly.
	Treatments []TaxTreatment `gorm:"-" json:"treatments,omitempty"`
} // @name Gain

// DisposalIncomeType reports what kind of income disposing of an asset of this
// tax class realizes.
//
// Government debt is bought at a discount and redeemed at par; the difference is
// accreted discount, which is interest income, not price appreciation - so a
// Treasury bill that matures produces interest even though the import records it
// as a sell. Whether something IS interest or capital is a fact about the
// instrument, upstream of how any jurisdiction taxes it, which is why it's
// decided here from the asset's class rather than by a per-jurisdiction rule.
//
// Held-to-maturity is assumed. A bond sold before maturity splits in principle
// into accrued discount plus a capital gain on price; that split isn't modeled.
func DisposalIncomeType(taxClass string) string {
	if taxClass == TaxClassGovernmentBond {
		return IncomeTypeInterest
	}
	return IncomeTypeCapitalGain
}
