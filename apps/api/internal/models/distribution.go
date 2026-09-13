package models

import (
	"time"

	"github.com/google/uuid"
)

// Distribution is one income event paid into an account - a dividend from a
// security or (later) interest from a cash/savings balance. It's a separate
// concern from trades (no lots, no price, no cost basis) and from capital gains
// (no disposal, no holding period), so it gets its own ledger. Like Gain it's
// derived from imports rather than user-authored, so it carries no soft delete.
type Distribution struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Category is the kind of income: "dividend" (from a security) or "interest"
	// (from a cash/savings balance, handled later).
	Category string `gorm:"not null;default:dividend" json:"category"`

	// HoldingID, Symbol, and AssetType identify the paying security. Symbol and
	// AssetType are unset for account-level income like bank interest, and
	// HoldingID is nil until a matching holding exists to link to.
	HoldingID *uuid.UUID `gorm:"type:uuid;index" json:"holding_id"`
	AccountID uuid.UUID  `gorm:"type:uuid;index" json:"account_id"`
	Symbol    string     `json:"symbol"`
	AssetType *string    `json:"asset_type"`

	PaymentDate time.Time `gorm:"type:date" json:"payment_date"`
	Amount      float64   `json:"amount"`

	// TransactionID is nil for a cash dividend or interest payment. It's reserved
	// for a reinvested dividend, which also opens a buy lot - not handled yet.
	TransactionID *uuid.UUID `gorm:"type:uuid;index" json:"transaction_id"`
} // @name Distribution

// ResolveTaxClass reports what kind of income this distribution represents, from
// the paying security's tax class. holding is that security, or nil when it
// isn't tracked - in which case the asset type recorded on the distribution
// itself is the best available signal.
//
// The qualified-vs-ordinary question this used to answer directly now belongs to
// the jurisdiction rules, which read this class: a distribution from an equity
// holding can qualify for a preferential federal rate, while money-market and
// government-bond income is always ordinary.
func (d Distribution) ResolveTaxClass(holding *Holding) string {
	if holding != nil {
		return holding.ResolveTaxClass()
	}
	if d.AssetType != nil {
		return defaultTaxClass(*d.AssetType)
	}
	return TaxClassOther
}

// ResolveIssuerJurisdiction reports which government issued the paying security's
// debt, or "" when there is none or it isn't tracked.
func (d Distribution) ResolveIssuerJurisdiction(holding *Holding) string {
	if holding != nil {
		return holding.ResolveIssuerJurisdiction()
	}
	if d.AssetType != nil {
		if code := defaultIssuerJurisdiction(*d.AssetType); code != nil {
			return *code
		}
	}
	return ""
}
