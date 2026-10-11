package models

import (
	"time"

	"github.com/google/uuid"
)

// TaxLot is one opening of a position - the shares a buy acquired, or the
// contracts a written option sold short - and how much of it is still open.
//
// It is derived, never authored, like RealizedEvent: every lot is rebuilt from
// the opening Transaction it points at, so transactions stay exactly as they
// were imported or entered. Anything that changes a lot after it opened - a
// sale depleting it, a split resizing it - is applied here, never written back
// onto the transaction. Rebuilds update lots in place by OpeningTransactionID,
// so a lot keeps its id across replays and references to it stay valid.
//
// Quantities are in today's shares: a lot opened before a split is sized as if
// the split had already happened, and the per-share basis falls out of
// CostBasis / Quantity. That keeps a lot comparable with the current position
// and price without remembering which splits it has seen.
//
// One lot per opening transaction today. Wash sales will need to adjust a lot
// after the fact - add the disallowed loss to CostBasis and tack the washed
// lot's holding period onto it - and a partly-replaced lot may need splitting,
// so OpeningTransactionID is deliberately not unique.
type TaxLot struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	HoldingID uuid.UUID `gorm:"type:uuid;not null;index" json:"holding_id"`
	AccountID uuid.UUID `gorm:"type:uuid;not null;index" json:"account_id"`

	// OpeningTransactionID is the trade that opened this lot (effect "open"): a
	// buy for a long lot, a sell-to-open for a written option. The trades that
	// closed it are reached through RealizedEvent.TaxLotID.
	OpeningTransactionID uuid.UUID `gorm:"type:uuid;not null;index" json:"opening_transaction_id"`

	// Direction is DirectionLong for bought shares or contracts, DirectionShort
	// for a written option, whose lot took premium in rather than paying it out.
	Direction string `gorm:"not null" json:"direction"`

	// AcquiredDate is when the lot was opened, and what FIFO/LIFO orders by.
	// A wash sale's tacked holding period should get its own column rather than
	// move this one, or it would reorder lots.
	AcquiredDate time.Time `gorm:"type:date;not null;index" json:"acquired_date"`

	// Quantity is the lot's full size and RemainingQuantity the part not yet
	// closed, both in today's shares (see the type comment).
	Quantity          float64 `gorm:"not null" json:"quantity"`
	RemainingQuantity float64 `gorm:"not null" json:"remaining_quantity"`

	// CostBasis is the whole lot's basis, commission and fees included and the
	// contract multiplier applied: what a long lot cost, or the net premium a
	// short lot took in. Unsigned - the direction says which side it sits on. A
	// split leaves it alone; only the share count changes.
	CostBasis float64 `gorm:"not null" json:"cost_basis"`

	// RealizedGains is the total gain realized from this lot so far: the sum of
	// the Amount of the realized events drawn from it, kept here so a lot list
	// needn't aggregate the ledger. Written alongside RemainingQuantity on every
	// close and reset with it on rebuild, so the two never disagree.
	RealizedGains float64 `gorm:"not null;default:0" json:"realized_gains"`
} // @name TaxLotRecord
