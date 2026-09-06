package models

import (
	"time"

	"github.com/google/uuid"
)

// Gain is one realized taxable event. For now only capital gains are recorded
// (dividends and interest come later), one row per (buy lot x sell) match - the
// granularity tax reporting needs, since a single sale can span a short-term and
// a long-term lot. It's a derived ledger, rebuilt when sells change, so no soft
// delete.
type Gain struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Category string `gorm:"not null;default:capital_gain" json:"category"`

	HoldingID *uuid.UUID `gorm:"type:uuid;index" json:"holding_id"`
	AccountID uuid.UUID  `gorm:"type:uuid;index" json:"account_id"`
	Symbol    string     `json:"symbol"`

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
} // @name Gain
