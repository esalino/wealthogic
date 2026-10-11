package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StockSplit is a split of a holding's shares: every OldShares held become
// NewShares, so a 2-for-1 is 1 -> 2 and a 1-for-8 reverse split is 8 -> 1.
//
// It is source data, the peer of Transaction: the user records it, and the
// lots are rebuilt around it. It is an event on the security, not on any one
// account, so it applies to every account's lots of the holding. Transactions
// are never rewritten for it - a buy of 2000 pre-split shares stays a buy of
// 2000 shares, and the lot it opened is what gets resized.
type StockSplit struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                 json:"-"`

	HoldingID uuid.UUID `gorm:"type:uuid;not null;index" json:"holding_id"`

	// EffectiveDate is the first day the shares trade split-adjusted (the
	// ex-date). A trade on this date is already in post-split shares.
	EffectiveDate time.Time `gorm:"type:date;not null" json:"effective_date"`

	OldShares float64 `gorm:"not null" json:"old_shares"`
	NewShares float64 `gorm:"not null" json:"new_shares"`
} // @name StockSplit

// Ratio is how many shares one pre-split share became.
func (s StockSplit) Ratio() float64 {
	if s.OldShares == 0 {
		return 1
	}
	return s.NewShares / s.OldShares
}
