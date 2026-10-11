package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Strategy statuses. A retired strategy keeps its trades and stats but isn't
// offered when starting a new trade.
const (
	StrategyStatusActive  = "active"
	StrategyStatusRetired = "retired"
)

// Rule kinds: what a rule governs - getting in, getting out, or how much is at
// stake.
const (
	RuleKindEntry = "entry"
	RuleKindExit  = "exit"
	RuleKindRisk  = "risk"
)

// Strategy is a named, repeatable way of trading, written down as a checklist
// of entry, exit and risk rules so a trade can be judged against it.
//
// Rules are plain text for now. They're the user's own checklist, not
// something evaluated against market data.
type Strategy struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                 json:"-"`

	Name        string `gorm:"not null"                 json:"name"`
	Description string `gorm:"type:text"                json:"description"`
	Status      string `gorm:"not null;default:active"  json:"status"`

	// Direction is DirectionLong, DirectionShort, or "both". Timeframe is free
	// text ("swing", "daily", "weeks") - descriptive only.
	Direction string `json:"direction"`
	Timeframe string `json:"timeframe"`

	Rules []StrategyRule `gorm:"foreignKey:StrategyID;constraint:OnDelete:CASCADE" json:"rules"`
} // @name Strategy

// StrategyRule is one item on a strategy's checklist.
//
// Rules are replaced wholesale when a strategy is edited, so they carry no soft
// delete: a trade never points at a live rule. It copies the rules into its own
// TradeRuleChecks when it starts, so editing a strategy can't rewrite how past
// trades were judged.
type StrategyRule struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	StrategyID uuid.UUID `gorm:"type:uuid;not null;index" json:"strategy_id"`
	Kind       string    `gorm:"not null"                 json:"kind"`
	Text       string    `gorm:"type:text;not null"       json:"text"`
	SortOrder  int       `gorm:"not null;default:0"       json:"sort_order"`
} // @name StrategyRule
