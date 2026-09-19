// Package portfolio holds shared portfolio-math used by both the HTTP handlers
// and the file import, kept here to avoid an import cycle between them.
package portfolio

import (
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/tax"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// absQuantity is a transaction's size in units, sign discarded - Fidelity signs
// quantity by trade direction (negative when sold), which the Effect and
// Direction fields already record.
func absQuantity(txn models.Transaction) float64 {
	if txn.Quantity == nil {
		return 0
	}
	if *txn.Quantity < 0 {
		return -*txn.Quantity
	}
	return *txn.Quantity
}

// unitPrice is a transaction's quoted price per share, zero when it has none -
// an expiration, which closes a position at no price at all.
func unitPrice(txn models.Transaction) float64 {
	if txn.Price == nil {
		return 0
	}
	return *txn.Price
}

// tradeValue is what `take` units of a trade are worth before costs: price is
// quoted per share, so an option contract covering 100 shares is worth a hundred
// times its quoted price.
func tradeValue(txn models.Transaction, take, multiplier float64) float64 {
	return take * unitPrice(txn) * multiplier
}

// feeShare allocates a trade's commission and fees pro-rata to the units taken
// from it, as tax rules require.
func feeShare(txn models.Transaction, take float64) float64 {
	qty := absQuantity(txn)
	if qty == 0 {
		return 0
	}
	return (take / qty) * (txn.Commission + txn.Fees)
}

// LotUnitCost is what one share of an opening trade cost, or took in, with the
// trade's commission and fees spread across the shares as tax rules require.
//
// Costs always work against you, so they add to a long lot's basis and subtract
// from the premium a written one received. The result is unsigned - it is a
// magnitude per share, and callers apply the sign that suits their direction.
func LotUnitCost(open models.Transaction, multiplier float64) float64 {
	qty := absQuantity(open)
	price := unitPrice(open)
	if qty == 0 || multiplier == 0 {
		return price
	}
	costs := (open.Commission + open.Fees) / (qty * multiplier)
	if open.Direction == models.DirectionShort {
		return price - costs
	}
	return price + costs
}

// openLots returns a holding's open lots on one side of the market
// (remaining_quantity > 0) for one account, in the account's cost-basis order
// (FIFO oldest-first, LIFO newest-first).
//
// Direction matters because a closing trade can only draw from lots it actually
// closes: buying back a written option must not consume a long lot of the same
// contract, and vice versa.
func openLots(tx *gorm.DB, holdingID, accountID uuid.UUID, direction, costBasisMethod string) ([]models.Transaction, error) {
	dateOrder, idOrder := "date ASC", "id ASC"
	if strings.EqualFold(costBasisMethod, "LIFO") {
		dateOrder, idOrder = "date DESC", "id DESC"
	}
	var lots []models.Transaction
	err := tx.Where("holding_id = ? AND account_id = ? AND effect = ? AND direction = ? AND remaining_quantity > 0",
		holdingID, accountID, models.EffectOpen, direction).
		Order(dateOrder).Order(idOrder).Find(&lots).Error
	return lots, err
}

// DepleteLots closes a position: it consumes units from the holding's open lots
// on the matching side of the market in the account's cost-basis order,
// decrementing each lot's remaining_quantity and writing a RealizedEvent per lot
// it draws from, plus that event's per-jurisdiction tax treatments. It returns
// the total realized gain and the quantity it could NOT fill (0 when there were
// enough units). Callers that must reject an over-close check the unfilled
// amount; lenient callers (imports) ignore it.
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

	// Load the holding for the canonical asset type/symbol on the event rows,
	// and for the contract multiplier its prices are quoted against.
	var holding models.Holding
	if err := tx.First(&holding, "id = ?", *closing.HoldingID).Error; err != nil {
		return 0, 0, err
	}
	multiplier := holding.Multiplier()

	// Direction says which side is being closed. Rows written before options
	// existed carry none and are all long.
	direction := closing.Direction
	if direction == "" {
		direction = models.DirectionLong
	}
	short := direction == models.DirectionShort

	closeQty := absQuantity(*closing)
	lots, err := openLots(tx, *closing.HoldingID, closing.AccountID, direction, costBasisMethod)
	if err != nil {
		return 0, 0, err
	}

	remaining := closeQty
	for i := range lots {
		if remaining <= 0 {
			break
		}
		lot := &lots[i]
		take := *lot.RemainingQuantity
		if take > remaining {
			take = remaining
		}

		// What the lot was opened for, and what closing it cost or returned.
		// Fees always work against you: they add to a cost and subtract from a
		// receipt, on both sides of the trade.
		openValue := tradeValue(*lot, take, multiplier)
		openFees := feeShare(*lot, take)
		closeValue := tradeValue(*closing, take, multiplier)
		closeFees := feeShare(*closing, take)

		var costBasis, proceeds float64
		if short {
			// Written: the premium came in at open, the buy-back is the cost.
			// An expiration closes at zero, leaving the whole premium as gain.
			proceeds = openValue - openFees
			costBasis = closeValue + closeFees
		} else {
			proceeds = closeValue - closeFees
			costBasis = openValue + openFees
		}
		gain := proceeds - costBasis

		// A written option's gain or loss is short-term however long it was
		// held (IRC 1234(b)), so only a long position earns a holding period.
		term := "short"
		if !short && !closing.Date.Before(lot.Date.AddDate(1, 0, 1)) {
			term = "long"
		}

		acquired := lot.Date
		g := models.RealizedEvent{
			// A disposal doesn't always realize a capital gain - redeeming a
			// discount instrument realizes interest - so the asset's tax class
			// decides, not the action that disposed of it.
			Category:         models.DisposalIncomeType(holding.ResolveTaxClass()),
			Origin:           models.OriginDisposal,
			HoldingID:        closing.HoldingID,
			AccountID:        closing.AccountID,
			Symbol:           holding.Symbol,
			AssetType:        holding.AssetType,
			TransactionID:    &closing.ID,
			LotTransactionID: &lot.ID,
			AcquiredDate:     &acquired,
			EventDate:        closing.Date,
			Quantity:         &take,
			CostBasis:        &costBasis,
			Proceeds:         &proceeds,
			Term:             term,
			Amount:           gain,
		}
		if err := tx.Create(&g).Error; err != nil {
			return 0, 0, err
		}

		// The event is realized now, so its tax treatment is settled now - by the
		// rules and the holding's tax attributes as they stand at realization.
		if err := applier.Apply(&g, &holding); err != nil {
			return 0, 0, err
		}

		newRemaining := *lot.RemainingQuantity - take
		if err := tx.Model(lot).Update("remaining_quantity", newRemaining).Error; err != nil {
			return 0, 0, err
		}

		realized += gain
		remaining -= take
	}
	return realized, remaining, nil
}

// RecalcHolding recomputes a holding's aggregates and saves them: position
// (quantity, cost basis, current value, unrealized gain) from its open buy lots,
// and realized gain from the Gain ledger. Dividend income isn't derived here yet.
func RecalcHolding(tx *gorm.DB, holding *models.Holding) error {
	// Open lots are the whole position. The transaction ledger is the only place
	// a position is derived from, so a holding with no lots is genuinely empty
	// here - which is how an un-imported transaction history makes itself
	// visible, rather than being papered over with a stale figure.
	var lots []models.Transaction
	if err := tx.Where("holding_id = ? AND effect = ? AND remaining_quantity > 0", holding.ID, models.EffectOpen).
		Find(&lots).Error; err != nil {
		return err
	}

	// Net the two sides: a written option is a liability, so its units and the
	// premium taken in count against the long side rather than adding to it. A
	// net-short position is negative on both counts, which is what it is.
	multiplier := holding.Multiplier()
	var quantity, costBasisTotal float64
	for i := range lots {
		rem := *lots[i].RemainingQuantity
		value := rem * LotUnitCost(lots[i], multiplier) * multiplier
		if lots[i].Direction == models.DirectionShort {
			quantity -= rem
			costBasisTotal -= value
			continue
		}
		quantity += rem
		costBasisTotal += value
	}

	holding.Quantity = quantity
	holding.CostBasisTotal = costBasisTotal
	if quantity != 0 {
		holding.AverageCostBasis = costBasisTotal / quantity
	} else {
		holding.AverageCostBasis = 0
	}

	holding.CurrentValue = quantity * holding.LastPrice * multiplier
	holding.GainUnrealizedAmount = holding.CurrentValue - costBasisTotal
	if costBasisTotal > 0 {
		holding.GainUnrealizedPercent = (holding.GainUnrealizedAmount / costBasisTotal) * 100
	} else {
		holding.GainUnrealizedPercent = 0
	}

	// Realized amounts come from this holding's disposals. Income is excluded:
	// a dividend has no cost basis, so folding it in would make the realized
	// percentage meaningless. A Treasury's return counts, though it's interest
	// rather than a capital gain - it still came from disposing of a lot.
	var disposals []models.RealizedEvent
	if err := tx.Where("holding_id = ? AND origin = ?", holding.ID, models.OriginDisposal).
		Find(&disposals).Error; err != nil {
		return err
	}
	var realizedAmount, realizedCostBasis float64
	for _, g := range disposals {
		realizedAmount += g.Amount
		if g.CostBasis != nil {
			realizedCostBasis += *g.CostBasis
		}
	}
	holding.GainRealizedAmount = realizedAmount
	if realizedCostBasis > 0 {
		holding.GainRealizedPercent = (realizedAmount / realizedCostBasis) * 100
	} else {
		holding.GainRealizedPercent = 0
	}

	return tx.Save(holding).Error
}
