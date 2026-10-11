package portfolio

import (
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Splits is one holding's split history. It is the single place a share count
// is carried across splits: lots, realized events and (later) trade fills all
// convert through it, so they can't disagree about what a pre-split share is
// worth today.
type Splits []models.StockSplit

// LoadSplits returns a holding's recorded splits.
func LoadSplits(tx *gorm.DB, holdingID uuid.UUID) (Splits, error) {
	var splits Splits
	err := tx.Where("holding_id = ?", holdingID).Order("effective_date ASC").Find(&splits).Error
	return splits, err
}

// Factor is how many of today's shares one share held on date has become: the
// product of every split that took effect after it. A split's effective date is
// its ex-date, when trading is already post-split, so a trade on that very date
// is not adjusted by it.
func (s Splits) Factor(date time.Time) float64 {
	factor := 1.0
	for _, split := range s {
		if split.EffectiveDate.After(date) {
			factor *= split.Ratio()
		}
	}
	return factor
}

// ToCurrent converts a share count as of date into today's shares.
func (s Splits) ToCurrent(quantity float64, date time.Time) float64 {
	return quantity * s.Factor(date)
}

// FromCurrent converts a count of today's shares back into the shares as they
// were on date - what a trade on that date actually moved.
func (s Splits) FromCurrent(quantity float64, date time.Time) float64 {
	return quantity / s.Factor(date)
}
