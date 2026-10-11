package models

import (
	"time"

	"github.com/google/uuid"
)

// Trade statuses. A trade is closed once every leg it opened is flat again.
const (
	TradeStatusOpen   = "open"
	TradeStatusClosed = "closed"
)

// Trade structures. Only single-leg trades are started from the UI today; the
// others name multi-leg positions the model already supports.
const (
	StructureSingle      = "single"
	StructureCoveredCall = "covered_call"
	StructureVertical    = "vertical"
	StructureOther       = "other"
)

// Fill sources. A manual fill was attached by the user; an auto fill was
// attached by the trade sync because it closed shares the trade had open.
const (
	FillSourceManual = "manual"
	FillSourceAuto   = "auto"
)

// Exit reasons a user can record on a closing fill.
const (
	ExitReasonTarget        = "target"
	ExitReasonStop          = "stop"
	ExitReasonTrailingStop  = "trailing_stop"
	ExitReasonTime          = "time"
	ExitReasonRule          = "rule"
	ExitReasonDiscretionary = "discretionary"
	ExitReasonExpired       = "expired"
	ExitReasonAssigned      = "assigned"
)

// ExitReasons is every valid exit reason, for validation.
var ExitReasons = []string{
	ExitReasonTarget, ExitReasonStop, ExitReasonTrailingStop, ExitReasonTime,
	ExitReasonRule, ExitReasonDiscretionary, ExitReasonExpired, ExitReasonAssigned,
}

// Trade is a journaled position: the fills that opened and closed it, the plan
// it was taken under, and how it turned out.
//
// It is a layer over the transaction ledger, not part of it. A trade measures a
// decision from entry to flat on an average-cost basis, while tax lots measure
// each acquisition until it's disposed of - so a trade with two scale-in buys
// spans two lots, and a sell the tax ledger matches FIFO to an older investment
// lot can still close this trade. Neither ledger reads the other.
//
// Trades are opt-in: one exists only because the user started it from an
// opening transaction. Unlike lots it is authored - it carries the plan and the
// notes - so it is never rebuilt wholesale. The sync re-derives only the auto
// fills and the cached figures below.
type Trade struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// A trade lives in one account. Underlying is the stock the legs are on -
	// the symbol itself for a stock, the option's underlying for a contract -
	// so a covered call's shares and calls belong to one trade.
	AccountID  uuid.UUID `gorm:"type:uuid;not null;index" json:"account_id"`
	Underlying string    `gorm:"not null;index"           json:"underlying"`
	Structure  string    `gorm:"not null;default:single"  json:"structure"`

	// Direction is the side of the first leg: long for a buy, short for a
	// written option.
	Direction string `gorm:"not null" json:"direction"`

	// Status, OpenedOn and ClosedOn are derived from the fills on every sync:
	// opened at the first entry, closed on the exit that left every leg flat.
	Status   string     `gorm:"not null;default:open;index" json:"status"`
	OpenedOn *time.Time `gorm:"type:date;index"             json:"opened_on"`
	ClosedOn *time.Time `gorm:"type:date"                   json:"closed_on"`

	StrategyID *uuid.UUID `gorm:"type:uuid;index" json:"strategy_id"`
	Strategy   *Strategy  `json:"strategy,omitempty"`

	// The plan. Prices are per share, as quoted. InitialStop is where the stop
	// was when the trade was taken and isn't meant to move - R is measured
	// against it - while CurrentStop tracks a trailing stop.
	//
	// PlannedRisk is the dollars at stake if the initial stop is hit. It's
	// derived from the stop for a single-leg trade, but stored because a
	// multi-leg position has no single stop price to derive it from.
	InitialStop *float64 `json:"initial_stop"`
	CurrentStop *float64 `json:"current_stop"`
	Target      *float64 `json:"target"`
	PlannedRisk *float64 `json:"planned_risk"`

	Notes  string `gorm:"type:text" json:"notes"`
	Rating *int   `json:"rating"` // 1-5, the user's grade of how the trade was executed

	// Cached figures, recomputed from the fills on every sync. Realized only:
	// what's still open is valued at read time against the holding's price
	// (UnrealizedPnL), since a reprice doesn't run the sync.
	//
	// OpenQuantity and the average prices are the first leg's, in today's
	// shares, so they line up with the holding's current price. EntryValue is
	// the price-only value of every entry, the capital the trade put to work.
	// GrossPnL is price-only; NetPnL takes off commission and fees.
	OpenQuantity  float64  `gorm:"not null;default:0"                  json:"open_quantity"`
	AvgEntryPrice float64  `gorm:"not null;default:0"                  json:"avg_entry_price"`
	AvgExitPrice  float64  `gorm:"not null;default:0"                  json:"avg_exit_price"`
	EntryValue    float64  `gorm:"not null;default:0"                  json:"entry_value"`
	GrossPnL      float64  `gorm:"column:gross_pnl;not null;default:0" json:"gross_pnl"`
	Fees          float64  `gorm:"not null;default:0"                  json:"fees"`
	NetPnL        float64  `gorm:"column:net_pnl;not null;default:0"   json:"net_pnl"`
	ReturnPercent *float64 `json:"return_percent"`

	// RMultiple is NetPnL over PlannedRisk, set once the trade is closed - a
	// partial result in R would read as a final one.
	RMultiple   *float64 `json:"r_multiple"`
	HoldingDays *int     `json:"holding_days"`

	UnrealizedPnL *float64 `gorm:"-" json:"unrealized_pnl"`

	Fills      []TradeFill      `gorm:"foreignKey:TradeID;constraint:OnDelete:CASCADE" json:"fills"`
	RuleChecks []TradeRuleCheck `gorm:"foreignKey:TradeID;constraint:OnDelete:CASCADE" json:"rule_checks"`
} // @name Trade

// TradeFill assigns some or all of a transaction to a trade. Whether it's an
// entry or an exit is the transaction's own Effect - an open adds to the trade,
// a close takes from it - so it isn't stored twice.
//
// Quantity is in the transaction's own shares, so it compares directly with
// the transaction it came from; the trade math converts through the holding's
// splits. A transaction can be split across trades, or partly left out of any,
// which is how a sell is divided between a trade and long-term shares of the
// same stock.
//
// Detached marks an auto fill the user removed. The row stays, so the next
// sync knows not to attach that transaction to this trade again; it counts for
// nothing in the trade's figures.
type TradeFill struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	TradeID       uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:idx_trade_fill" json:"trade_id"`
	TransactionID uuid.UUID    `gorm:"type:uuid;not null;uniqueIndex:idx_trade_fill;index" json:"transaction_id"`
	Transaction   *Transaction `json:"transaction,omitempty"`

	Quantity   float64 `gorm:"not null"                 json:"quantity"`
	Source     string  `gorm:"not null;default:manual"  json:"source"`
	Detached   bool    `gorm:"not null;default:false"   json:"detached"`
	ExitReason string  `json:"exit_reason"`
} // @name TradeFill

// TradeRuleCheck is one strategy rule as it stood when the trade started, and
// whether the trade followed it. Met is nil until answered - exit rules can't
// be judged until the trade is out.
type TradeRuleCheck struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	TradeID   uuid.UUID `gorm:"type:uuid;not null;index" json:"trade_id"`
	Kind      string    `gorm:"not null"                 json:"kind"`
	Text      string    `gorm:"type:text;not null"       json:"text"`
	SortOrder int       `gorm:"not null;default:0"       json:"sort_order"`
	Met       *bool     `json:"met"`
} // @name TradeRuleCheck
