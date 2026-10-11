package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Account represents a brokerage or bank account.
type Account struct {
	ID               uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index"                                           json:"-"`
	AccountName      string         `gorm:"not null"                                        json:"account_name"`
	AccountType      string         `gorm:"not null"                                        json:"account_type"`
	TaxType          string         `gorm:"not null"                                        json:"tax_type"`
	DefaultCostBasis string         `gorm:"not null;default:FIFO"                           json:"default_cost_basis"`
	Balance          float64        `json:"balance"`

	// CashBalance is the account's uninvested cash (core money market and
	// pending activity), as the broker last reported it. Positions are derived
	// from tax lots, but cash never forms a lot, so it can only come from an
	// import. CashBalanceAsOf is the date the broker's figure describes; nil
	// means no import has reported cash, and an account with neither cash nor
	// lots keeps a hand-entered Balance (see portfolio.RecalcAccountBalance).
	CashBalance     float64    `gorm:"not null;default:0" json:"cash_balance"`
	CashBalanceAsOf *time.Time `gorm:"type:date"          json:"cash_balance_as_of"`

	Transactions []Transaction `json:"transactions,omitempty"`
	Owners       []User        `gorm:"many2many:account_owners;"                       json:"owners,omitempty"`
} // @name Account
