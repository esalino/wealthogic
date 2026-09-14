package portfolio

import (
	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/tax"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RealizeDistribution rebuilds the realized events derived from one income
// payment, together with their tax treatments.
//
// It mirrors what rebuildHolding does for trades: the source record is the
// user's, the ledger below it is derived, so any create, edit, or delete of a
// distribution replays this rather than trying to patch the ledger in place.
// Safe to call for a soft-deleted distribution - it clears the events and stops.
func RealizeDistribution(tx *gorm.DB, dist *models.Distribution, applier *tax.Applier) error {
	// Clear first: treatments cascade from the events, and a replay must not
	// leave the previous version's rows behind.
	if err := DeleteDistributionEvents(tx, dist.ID); err != nil {
		return err
	}
	if dist.DeletedAt.Valid {
		return nil
	}

	// The paying security, when it's tracked. A dividend can predate the first
	// buy of the security that paid it, so fall back to the symbol.
	var holding *models.Holding
	var found models.Holding
	switch {
	case dist.HoldingID != nil:
		if err := tx.First(&found, "id = ?", *dist.HoldingID).Error; err == nil {
			holding = &found
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
	case dist.Symbol != "":
		if err := tx.First(&found, "symbol = ?", dist.Symbol).Error; err == nil {
			holding = &found
		} else if err != gorm.ErrRecordNotFound {
			return err
		}
	}

	holdingID := dist.HoldingID
	assetType := ""
	if holding != nil {
		holdingID = &holding.ID
		assetType = holding.AssetType
	} else if dist.AssetType != nil {
		assetType = *dist.AssetType
	}

	// One event per payment today. The ledger allows several per source, which
	// is what a fund splitting one payment by character would need - part of a
	// Treasury money-market distribution being federal-obligation interest and
	// the rest not - but that split isn't modeled yet.
	ev := models.RealizedEvent{
		Category:       dist.IncomeType(),
		Origin:         models.OriginDistribution,
		HoldingID:      holdingID,
		AccountID:      dist.AccountID,
		Symbol:         dist.Symbol,
		AssetType:      assetType,
		EventDate:      dist.PaymentDate,
		Amount:         dist.Amount,
		DistributionID: &dist.ID,
	}
	if err := tx.Create(&ev).Error; err != nil {
		return err
	}
	return applier.Apply(&ev, holding)
}

// DeleteDistributionEvents removes the realized events derived from one income
// payment, and the treatments hanging off them.
func DeleteDistributionEvents(tx *gorm.DB, distributionID uuid.UUID) error {
	if err := deleteTreatmentsFor(tx, tx.Model(&models.RealizedEvent{}).
		Select("id").Where("distribution_id = ?", distributionID)); err != nil {
		return err
	}
	return tx.Where("distribution_id = ?", distributionID).Delete(&models.RealizedEvent{}).Error
}

// DeleteHoldingDisposalEvents removes the events a holding's sells produced, and
// their treatments. Income events are left alone: they derive from distributions,
// which a trade replay knows nothing about and must never destroy.
func DeleteHoldingDisposalEvents(tx *gorm.DB, holdingID uuid.UUID) error {
	scope := tx.Model(&models.RealizedEvent{}).Select("id").
		Where("holding_id = ? AND origin = ?", holdingID, models.OriginDisposal)
	if err := deleteTreatmentsFor(tx, scope); err != nil {
		return err
	}
	return tx.Where("holding_id = ? AND origin = ?", holdingID, models.OriginDisposal).
		Delete(&models.RealizedEvent{}).Error
}

// deleteTreatmentsFor removes the treatments belonging to whichever events the
// given subquery selects.
func deleteTreatmentsFor(tx *gorm.DB, eventIDs *gorm.DB) error {
	return tx.Where("realized_event_id IN (?)", eventIDs).Delete(&models.TaxTreatment{}).Error
}

// RetaxHolding re-derives the tax consequences of everything already realized
// from a holding, for when the holding's own tax attributes change. Correcting a
// tax class has to reach events already recorded, or the old classification
// stays baked into the ledger.
//
// It reclassifies rather than replays: changing what an asset IS doesn't change
// what a sale was worth, only what kind of income it produced and how each
// jurisdiction treats it.
func RetaxHolding(tx *gorm.DB, holdingID uuid.UUID) error {
	applier, err := tax.NewApplier(tx)
	if err != nil {
		return err
	}

	var holding models.Holding
	if err := tx.First(&holding, "id = ?", holdingID).Error; err != nil {
		return err
	}

	// Income can carry the symbol but no holding id, for a dividend paid before
	// the security's first buy - match on either so nothing is missed.
	q := tx.Where("holding_id = ?", holdingID)
	if holding.Symbol != "" {
		q = tx.Where("holding_id = ? OR symbol = ?", holdingID, holding.Symbol)
	}
	var events []models.RealizedEvent
	if err := q.Find(&events).Error; err != nil {
		return err
	}

	disposalCategory := models.DisposalIncomeType(holding.ResolveTaxClass())
	for i := range events {
		ev := &events[i]
		if ev.Origin == models.OriginDisposal && ev.Category != disposalCategory {
			ev.Category = disposalCategory
			if err := tx.Model(ev).Update("category", disposalCategory).Error; err != nil {
				return err
			}
		}
		if err := applier.Apply(ev, &holding); err != nil {
			return err
		}
	}
	return nil
}
