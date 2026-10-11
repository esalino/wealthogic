package portfolio

import (
	"errors"
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/tax"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrInsufficientShares is returned by a strict rebuild when a closing trade
// asks for more units than the account's open lots hold.
var ErrInsufficientShares = errors.New("not enough shares to close")

// OpenBasis is the basis of the still-open part of a lot: its whole basis
// prorated by what remains. Unsigned, like the lot's CostBasis.
func OpenBasis(lot models.TaxLot) float64 {
	if lot.Quantity == 0 {
		return 0
	}
	return lot.CostBasis * lot.RemainingQuantity / lot.Quantity
}

// lotFor sizes the lot an opening trade forms, in today's shares, with its
// whole basis. It fills in a fresh lot or resets an existing one to full.
func lotFor(lot *models.TaxLot, open *models.Transaction, holding *models.Holding, splits Splits) {
	direction := open.Direction
	if direction == "" {
		direction = models.DirectionLong
	}
	multiplier := holding.Multiplier()
	traded := absQuantity(*open)
	quantity := splits.ToCurrent(traded, open.Date)

	lot.HoldingID = holding.ID
	lot.AccountID = open.AccountID
	lot.OpeningTransactionID = open.ID
	lot.Direction = direction
	lot.AcquiredDate = open.Date
	lot.Quantity = quantity
	lot.RemainingQuantity = quantity
	lot.CostBasis = traded * LotUnitCost(*open, multiplier) * multiplier
	lot.RealizedGains = 0
}

// OpenLot records the tax lot an opening trade forms. The trade must already
// be saved and linked to its holding. If the trade already has a lot, it is
// reset to full rather than duplicated, so calling this twice is harmless.
func OpenLot(tx *gorm.DB, open *models.Transaction) error {
	if open.HoldingID == nil || open.Quantity == nil {
		return nil
	}
	var holding models.Holding
	if err := tx.First(&holding, "id = ?", *open.HoldingID).Error; err != nil {
		return err
	}
	splits, err := LoadSplits(tx, holding.ID)
	if err != nil {
		return err
	}

	var lot models.TaxLot
	err = tx.Where("opening_transaction_id = ?", open.ID).First(&lot).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	lotFor(&lot, open, &holding, splits)
	return tx.Save(&lot).Error
}

// openLots returns a holding's open lots on one side of the market for one
// account, in the account's cost-basis order (FIFO oldest-first, LIFO
// newest-first). The opening transaction's id breaks ties within a day, since
// UUIDv7 ids follow insertion order.
//
// Direction matters because a closing trade can only draw from lots it actually
// closes: buying back a written option must not consume a long lot of the same
// contract, and vice versa.
func openLots(tx *gorm.DB, holdingID, accountID uuid.UUID, direction, costBasisMethod string) ([]models.TaxLot, error) {
	dateOrder, idOrder := "acquired_date ASC", "opening_transaction_id ASC"
	if strings.EqualFold(costBasisMethod, "LIFO") {
		dateOrder, idOrder = "acquired_date DESC", "opening_transaction_id DESC"
	}
	var lots []models.TaxLot
	err := tx.Where("holding_id = ? AND account_id = ? AND direction = ? AND remaining_quantity > ?",
		holdingID, accountID, direction, quantityEpsilon).
		Order(dateOrder).Order(idOrder).Find(&lots).Error
	return lots, err
}

// DepleteLots closes a position: it consumes units from the holding's open lots
// on the matching side of the market in the account's cost-basis order,
// decrementing each lot's remaining quantity and writing a RealizedEvent per lot
// it draws from, plus that event's per-jurisdiction tax treatments. It returns
// the total realized gain and the quantity it could NOT fill (0 when there were
// enough units), in the closing trade's own shares. Callers that must reject an
// over-close check the unfilled amount; lenient callers (imports) ignore it.
//
// Long and short close the same way, with the sides swapped: a long lot supplies
// the basis and the closing trade the proceeds, while a written option supplies
// the proceeds up front (the premium) and the closing trade the cost of buying
// it back. Gain is proceeds minus basis either way.
//
// applier is shared across a batch so the tax rules are loaded once; callers
// replaying a whole holding pass the same one for every trade.
func DepleteLots(tx *gorm.DB, closing *models.Transaction, costBasisMethod string, applier *tax.Applier) (realized, unfilled float64, err error) {
	if closing.HoldingID == nil || closing.Quantity == nil {
		return 0, 0, nil
	}
	var holding models.Holding
	if err := tx.First(&holding, "id = ?", *closing.HoldingID).Error; err != nil {
		return 0, 0, err
	}
	splits, err := LoadSplits(tx, holding.ID)
	if err != nil {
		return 0, 0, err
	}
	return depleteLots(tx, closing, &holding, splits, costBasisMethod, applier)
}

func depleteLots(tx *gorm.DB, closing *models.Transaction, holding *models.Holding, splits Splits, costBasisMethod string, applier *tax.Applier) (realized, unfilled float64, err error) {
	multiplier := holding.Multiplier()

	// Direction says which side is being closed. Rows written before options
	// existed carry none and are all long.
	direction := closing.Direction
	if direction == "" {
		direction = models.DirectionLong
	}
	short := direction == models.DirectionShort

	lots, err := openLots(tx, holding.ID, closing.AccountID, direction, costBasisMethod)
	if err != nil {
		return 0, 0, err
	}

	// Lots are in today's shares and the trade in the shares of its own day, so
	// match in today's and convert each fill back for the trade's side of the
	// figures.
	remaining := splits.ToCurrent(absQuantity(*closing), closing.Date)
	for i := range lots {
		if remaining <= quantityEpsilon {
			break
		}
		lot := &lots[i]
		take := lot.RemainingQuantity
		if take > remaining {
			take = remaining
		}
		traded := splits.FromCurrent(take, closing.Date)

		// The lot's share of its own basis, and what closing it cost or
		// returned. Fees always work against you: they add to a cost and
		// subtract from a receipt, on both sides of the trade.
		lotValue := lot.CostBasis * take / lot.Quantity
		closeValue := tradeValue(*closing, traded, multiplier)
		closeFees := feeShare(*closing, traded)

		var costBasis, proceeds float64
		if short {
			// Written: the premium came in at open, the buy-back is the cost.
			// An expiration closes at zero, leaving the whole premium as gain.
			proceeds = lotValue
			costBasis = closeValue + closeFees
		} else {
			proceeds = closeValue - closeFees
			costBasis = lotValue
		}
		gain := proceeds - costBasis

		// A written option's gain or loss is short-term however long it was
		// held (IRC 1234(b)), so only a long position earns a holding period.
		term := "short"
		if !short && !closing.Date.Before(lot.AcquiredDate.AddDate(1, 0, 1)) {
			term = "long"
		}

		acquired := lot.AcquiredDate
		taxLotID := lot.ID
		g := models.RealizedEvent{
			// A disposal doesn't always realize a capital gain - redeeming a
			// discount instrument realizes interest - so the asset's tax class
			// decides, not the action that disposed of it.
			Category:      models.DisposalIncomeType(holding.ResolveTaxClass()),
			Origin:        models.OriginDisposal,
			HoldingID:     closing.HoldingID,
			AccountID:     closing.AccountID,
			Symbol:        holding.Symbol,
			AssetType:     holding.AssetType,
			TransactionID: &closing.ID,
			TaxLotID:      &taxLotID,
			AcquiredDate:  &acquired,
			EventDate:     closing.Date,
			Quantity:      &traded,
			CostBasis:     &costBasis,
			Proceeds:      &proceeds,
			Term:          term,
			Amount:        gain,
		}
		if err := tx.Create(&g).Error; err != nil {
			return 0, 0, err
		}

		// The event is realized now, so its tax treatment is settled now - by the
		// rules and the holding's tax attributes as they stand at realization.
		if err := applier.Apply(&g, holding); err != nil {
			return 0, 0, err
		}

		lot.RemainingQuantity -= take
		lot.RealizedGains += gain
		if err := tx.Model(lot).Updates(map[string]any{
			"remaining_quantity": lot.RemainingQuantity,
			"realized_gains":     lot.RealizedGains,
		}).Error; err != nil {
			return 0, 0, err
		}

		realized += gain
		remaining -= take
	}
	if remaining < quantityEpsilon {
		remaining = 0
	}
	return realized, splits.FromCurrent(remaining, closing.Date), nil
}

// RebuildHolding replays a holding's whole lot ledger from its transactions and
// splits: every lot is reset to full, every disposal event is dropped, and the
// opening and closing trades are replayed oldest first, so each close draws
// only from lots that existed when it happened. Then the aggregates are
// recomputed. A single close's depletion can't be reversed in isolation, so any
// change to a holding's trades or splits rebuilds the whole holding.
//
// Lots are updated in place, matched by their opening transaction, so a lot
// keeps its id across rebuilds; lots whose transaction is gone are removed.
//
// When strict is set, a close that can't be fully filled returns
// ErrInsufficientShares so the caller can reject the change. Changes that can
// only free shares (deleting a sell) pass strict=false so they never fail.
func RebuildHolding(tx *gorm.DB, holdingID uuid.UUID, strict bool) error {
	var holding models.Holding
	if err := tx.First(&holding, "id = ?", holdingID).Error; err != nil {
		return err
	}
	splits, err := LoadSplits(tx, holdingID)
	if err != nil {
		return err
	}

	// Clear the events this holding's closes produced, and their treatments;
	// the replay recreates both. Keyed by holding_id, so it also drops rows from
	// trades that were soft-deleted.
	//
	// Scoped to disposals: income events in the same ledger derive from
	// distributions, which this replay cannot reconstruct, so wiping them here
	// would destroy records that only exist because a user or an import created
	// them.
	if err := DeleteHoldingDisposalEvents(tx, holdingID); err != nil {
		return err
	}

	// Empty every lot before the replay: a lot only refills when its opening
	// trade comes round, so a close can never draw on one that opens later.
	if err := tx.Model(&models.TaxLot{}).Where("holding_id = ?", holdingID).
		Updates(map[string]any{"remaining_quantity": 0, "realized_gains": 0}).Error; err != nil {
		return err
	}
	var existing []models.TaxLot
	if err := tx.Where("holding_id = ?", holdingID).Find(&existing).Error; err != nil {
		return err
	}
	lotsByTxn := make(map[uuid.UUID]*models.TaxLot, len(existing))
	for i := range existing {
		lotsByTxn[existing[i].OpeningTransactionID] = &existing[i]
	}

	var trades []models.Transaction
	if err := tx.Where("holding_id = ? AND effect IN ? AND quantity IS NOT NULL",
		holdingID, []string{models.EffectOpen, models.EffectClose}).
		Order("date ASC").Order("id ASC").Find(&trades).Error; err != nil {
		return err
	}

	// One applier for the whole replay: every close is evaluated against the
	// same rules, so they're loaded once rather than per trade.
	applier, err := tax.NewApplier(tx)
	if err != nil {
		return err
	}

	costBasisMethod := map[uuid.UUID]string{}
	kept := map[uuid.UUID]bool{}
	for i := range trades {
		trade := &trades[i]

		if trade.Effect == models.EffectOpen {
			lot, ok := lotsByTxn[trade.ID]
			if !ok {
				lot = &models.TaxLot{}
			}
			lotFor(lot, trade, &holding, splits)
			if err := tx.Save(lot).Error; err != nil {
				return err
			}
			kept[lot.ID] = true
			continue
		}

		method, ok := costBasisMethod[trade.AccountID]
		if !ok {
			var account models.Account
			if err := tx.First(&account, "id = ?", trade.AccountID).Error; err == nil {
				method = account.DefaultCostBasis
			}
			costBasisMethod[trade.AccountID] = method
		}
		realized, unfilled, err := depleteLots(tx, trade, &holding, splits, method, applier)
		if err != nil {
			return err
		}
		if strict && unfilled > quantityEpsilon {
			return ErrInsufficientShares
		}
		if err := tx.Model(trade).Update("realized_gains", realized).Error; err != nil {
			return err
		}
	}

	for _, lot := range existing {
		if !kept[lot.ID] {
			if err := tx.Delete(&models.TaxLot{}, "id = ?", lot.ID).Error; err != nil {
				return err
			}
		}
	}

	return RecalcHolding(tx, &holding)
}
