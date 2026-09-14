package models

import (
	"time"

	"github.com/google/uuid"
)

// Origins a realized event can be derived from. Exactly one of the matching
// source ids is set.
const (
	OriginDisposal     = "disposal"     // a sell consuming a tax lot
	OriginDistribution = "distribution" // an income payment received
)

// RealizedEvent is one taxable event, realized in a tax year: a disposal that
// consumed a tax lot, or income that was paid. It is the single ledger the tax
// rules run against and the Tax Center reads.
//
// It is derived, never authored: every row is rebuilt from its source record (a
// Transaction or a Distribution), so the whole table can be wiped and replayed
// and carries no soft delete. That uniformity is the point - the source records
// are the ones users edit, and nothing here needs protecting from a rebuild.
//
// One source can realize several events. A single sell produces one row per lot
// it draws from, since a sale can span a short-term and a long-term lot; and a
// fund distribution can split by character, where part of a Treasury money-market
// payment is federal-obligation interest and the rest is not.
type RealizedEvent struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Category is what was realized: IncomeTypeCapitalGain, IncomeTypeInterest,
	// or IncomeTypeDividend. Origin is which kind of source produced it.
	Category string `gorm:"not null;index" json:"category"`
	Origin   string `gorm:"not null;index" json:"origin"`

	Symbol    string `gorm:"index" json:"symbol"`
	AssetType string `json:"asset_type"`

	HoldingID *uuid.UUID `gorm:"type:uuid;index" json:"holding_id"`
	AccountID uuid.UUID  `gorm:"type:uuid;index" json:"account_id"`

	// EventDate is when it was realized - the sell's date, or the payment date.
	// One column for both so the ledger sorts and filters by time as a whole.
	EventDate time.Time `gorm:"type:date;index" json:"event_date"`
	Amount    float64   `json:"amount"`

	// Source links. Two real foreign keys rather than a polymorphic pair, so
	// each stays a genuine reference the database can enforce. TransactionID is
	// the realizing sell and LotTransactionID the buy that supplied the shares;
	// DistributionID is the income record. Exactly one origin's ids are set.
	TransactionID    *uuid.UUID `gorm:"type:uuid;index" json:"transaction_id"`
	LotTransactionID *uuid.UUID `gorm:"type:uuid;index" json:"lot_transaction_id"`
	DistributionID   *uuid.UUID `gorm:"type:uuid;index" json:"distribution_id"`

	// Lot detail, set only for a disposal. Nil on income, which has no lot
	// behind it - distinct from a cost basis that happens to be zero.
	AcquiredDate *time.Time `gorm:"type:date" json:"acquired_date"`
	Quantity     *float64   `json:"quantity"`
	CostBasis    *float64   `json:"cost_basis"`
	Proceeds     *float64   `json:"proceeds"`
	Term         string     `json:"term"` // "short" | "long"; empty for income

	// Treatments is how each jurisdiction taxes this event. A real association
	// now that the ledger is one table, so it preloads like any other.
	Treatments []TaxTreatment `gorm:"foreignKey:RealizedEventID;constraint:OnDelete:CASCADE" json:"treatments,omitempty"`
} // @name RealizedEvent

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
