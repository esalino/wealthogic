// Package portfolio holds shared portfolio-math used by both the HTTP handlers
// and the file import, kept here to avoid an import cycle between them.
package portfolio

import (
	"math"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// quantityEpsilon is the point below which a position counts as flat. Depleting
// a lot subtracts floats, so selling out of a fractional holding can leave a
// residue many orders of magnitude smaller than any real share count.
const quantityEpsilon = 1e-9

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

// ApplyPrice puts a newly quoted price on a holding and recomputes what follows
// from it: market value, and the unrealized gain against the basis already
// recorded. asOf is what the price is as of; a zero time leaves the existing
// stamp alone rather than claiming the price is current.
//
// The position itself is untouched. Quantity and cost basis come from the
// ledger, and a reprice knows nothing about either - it must not overwrite the
// figures of a holding whose transactions haven't been imported.
func ApplyPrice(holding *models.Holding, price float64, asOf time.Time) {
	holding.LastPrice = price
	if !asOf.IsZero() {
		at := asOf.UTC()
		holding.LastPriceUpdatedAt = &at
	}
	reprice(holding)
}

// reprice recomputes the figures that fall out of a holding's price and the
// position it already has. It is the one place that math lives, so a reprice
// and a full recompute can't drift apart.
func reprice(holding *models.Holding) {
	holding.CurrentValue = holding.Quantity * holding.LastPrice * holding.Multiplier()
	holding.GainUnrealizedAmount = holding.CurrentValue - holding.CostBasisTotal
	if holding.CostBasisTotal > 0 {
		holding.GainUnrealizedPercent = (holding.GainUnrealizedAmount / holding.CostBasisTotal) * 100
	} else {
		holding.GainUnrealizedPercent = 0
	}
}

// RecalcHolding recomputes a holding's aggregates and saves them: position
// (quantity, cost basis, current value, unrealized gain) from its open tax
// lots, and realized gain from the realized-event ledger. Dividend income isn't
// derived here yet.
func RecalcHolding(tx *gorm.DB, holding *models.Holding) error {
	// Open lots are the whole position. The transaction ledger is the only place
	// a position is derived from, so a holding with no lots is genuinely empty
	// here - which is how an un-imported transaction history makes itself
	// visible, rather than being papered over with a stale figure.
	var lots []models.TaxLot
	if err := tx.Where("holding_id = ? AND remaining_quantity > ?", holding.ID, quantityEpsilon).
		Find(&lots).Error; err != nil {
		return err
	}

	// Net the two sides: a written option is a liability, so its units and the
	// premium taken in count against the long side rather than adding to it. A
	// net-short position is negative on both counts, which is what it is.
	var quantity, costBasisTotal float64
	for i := range lots {
		rem := lots[i].RemainingQuantity
		value := OpenBasis(lots[i])
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

	reprice(holding)

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

	// Only a flat position can be closed, and only the ledger can say it was
	// closed rather than never loaded - so the count is worth a query only once
	// the quantity is already flat.
	var closings int64
	if math.Abs(quantity) < quantityEpsilon {
		if err := tx.Model(&models.Transaction{}).
			Where("holding_id = ? AND effect = ?", holding.ID, models.EffectClose).
			Count(&closings).Error; err != nil {
			return err
		}
	}
	holding.Status = DeriveStatus(quantity, closings)

	return tx.Save(holding).Error
}

// DeriveStatus decides a holding's status from its position and its ledger.
//
// A position that a sale took to zero is closed. A zero quantity alone doesn't
// say that: a holding imported without its transaction history has no lots
// either, and that's an absent ledger rather than an exited position - which is
// why a closing transaction has to exist to have done the closing. The rule runs
// both ways, so deleting that sale or buying back in reopens the holding; the
// status follows the ledger instead of latching.
func DeriveStatus(quantity float64, closings int64) string {
	if closings > 0 && math.Abs(quantity) < quantityEpsilon {
		return models.HoldingStatusClosed
	}
	return models.HoldingStatusOpen
}
