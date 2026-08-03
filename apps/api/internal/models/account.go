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

	Transactions []Transaction `json:"transactions,omitempty"`
	Owners       []User        `gorm:"many2many:account_owners;"                       json:"owners,omitempty"`
} // @name Account
