// Package portfolio holds shared portfolio-math used by both the HTTP handlers
// and the file import, kept here to avoid an import cycle between them.
package portfolio

import (
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// lotBasisPerShare is a buy's cost basis per share, including its commission and
// fees spread across the shares (as tax rules require).
func lotBasisPerShare(buy models.Transaction) float64 {
	qty := 0.0
	if buy.Quantity != nil {
		qty = *buy.Quantity
	}
	price := 0.0
	if buy.Price != nil {
		price = *buy.Price
	}
	if qty == 0 {
		return price
	}
	return price + (buy.Commission+buy.Fees)/qty
}

// openLots returns a holding's open stock-buy transactions (remaining_quantity >
// 0) for one account, in the account's cost-basis order (FIFO oldest-first,
// LIFO newest-first). A stock buy doubles as a tax lot.
func openLots(tx *gorm.DB, holdingID, accountID uuid.UUID, costBasisMethod string) ([]models.Transaction, error) {
	dateOrder, idOrder := "date ASC", "id ASC"
	if strings.EqualFold(costBasisMethod, "LIFO") {
		dateOrder, idOrder = "date DESC", "id DESC"
	}
	var buys []models.Transaction
	err := tx.Where("holding_id = ? AND account_id = ? AND LOWER(action) = ? AND remaining_quantity > 0", holdingID, accountID, "buy").
		Order(dateOrder).Order(idOrder).Find(&buys).Error
	return buys, err
}

// DepleteLots consumes the sell's shares from the holding's open buy lots in the
// account's cost-basis order, decrementing each lot's remaining_quantity and
// writing a Gain (capital-gain) row per lot it draws from. It returns the total
// realized gain and the quantity it could NOT fill (0 when there were enough
// shares). Callers that must reject an over-sell check the unfilled amount;
// lenient callers (imports) ignore it.
func DepleteLots(tx *gorm.DB, sell *models.Transaction, costBasisMethod string) (realized, unfilled float64, err error) {
	if sell.HoldingID == nil || sell.Quantity == nil || sell.Price == nil {
		return 0, 0, nil
	}

	// Sells may be recorded with a negative quantity (Fidelity's convention).
	sellQty := *sell.Quantity
	if sellQty < 0 {
		sellQty = -sellQty
	}
	sellPrice := *sell.Price
	sellFees := sell.Commission + sell.Fees

	buys, err := openLots(tx, *sell.HoldingID, sell.AccountID, costBasisMethod)
	if err != nil {
		return 0, 0, err
	}

	remaining := sellQty
	for i := range buys {
		if remaining <= 0 {
			break
		}
		buy := &buys[i]
		take := *buy.RemainingQuantity
		if take > remaining {
			take = remaining
		}

		costBasis := take * lotBasisPerShare(*buy)
		// Proceeds are net of the sell's fees, allocated pro-rata to the shares.
		var feeShare float64
		if sellQty > 0 {
			feeShare = (take / sellQty) * sellFees
		}
		proceeds := take*sellPrice - feeShare
		gain := proceeds - costBasis

		term := "short"
		if !sell.Date.Before(buy.Date.AddDate(1, 0, 1)) {
			term = "long" // held more than one year
		}

		g := models.Gain{
			Category:         "capital_gain",
			HoldingID:        sell.HoldingID,
			AccountID:        sell.AccountID,
			Symbol:           sell.Symbol,
			TransactionID:    sell.ID,
			LotTransactionID: &buy.ID,
			AcquiredDate:     buy.Date,
			RealizedDate:     sell.Date,
			Quantity:         take,
			CostBasis:        costBasis,
			Proceeds:         proceeds,
			Term:             term,
			Amount:           gain,
		}
		if err := tx.Create(&g).Error; err != nil {
			return 0, 0, err
		}

		newRemaining := *buy.RemainingQuantity - take
		if err := tx.Model(buy).Update("remaining_quantity", newRemaining).Error; err != nil {
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
	var buys []models.Transaction
	if err := tx.Where("holding_id = ? AND LOWER(action) = ? AND remaining_quantity > 0", holding.ID, "buy").Find(&buys).Error; err != nil {
		return err
	}

	var quantity, costBasisTotal float64
	for i := range buys {
		rem := *buys[i].RemainingQuantity
		quantity += rem
		costBasisTotal += rem * lotBasisPerShare(buys[i])
	}

	holding.Quantity = quantity
	holding.CostBasisTotal = costBasisTotal
	if quantity > 0 {
		holding.AverageCostBasis = costBasisTotal / quantity
	} else {
		holding.AverageCostBasis = 0
	}

	holding.CurrentValue = quantity * holding.LastPrice
	holding.GainUnrealizedAmount = holding.CurrentValue - costBasisTotal
	if costBasisTotal > 0 {
		holding.GainUnrealizedPercent = (holding.GainUnrealizedAmount / costBasisTotal) * 100
	} else {
		holding.GainUnrealizedPercent = 0
	}

	// Realized gains come straight from the Gain ledger for this holding.
	var gains []models.Gain
	if err := tx.Where("holding_id = ? AND category = ?", holding.ID, "capital_gain").Find(&gains).Error; err != nil {
		return err
	}
	var realizedAmount, realizedCostBasis float64
	for _, g := range gains {
		realizedAmount += g.Amount
		realizedCostBasis += g.CostBasis
	}
	holding.GainRealizedAmount = realizedAmount
	if realizedCostBasis > 0 {
		holding.GainRealizedPercent = (realizedAmount / realizedCostBasis) * 100
	} else {
		holding.GainRealizedPercent = 0
	}

	return tx.Save(holding).Error
}
