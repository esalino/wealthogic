// Package portfolio holds shared portfolio-math used by both the HTTP handlers
// and the file import, kept here to avoid an import cycle between them.
package portfolio

import (
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LotAllocation records that a sell drew `Quantity` shares from a specific tax
// lot. Callers turn these into TaxLotTransaction rows so a disposal can be
// resolved lot-by-lot for taxes.
type LotAllocation struct {
	TaxLotID uuid.UUID
	Quantity float64
}

// DepleteLots consumes `quantity` shares from a holding's open lots in the given
// account, in the account's cost-basis order (FIFO oldest-first by default,
// LIFO newest-first), reducing each lot's remaining quantity and saving it. It
// returns the realized gain against sellPrice, the quantity it could NOT fill
// (0 when there were enough shares), and the per-lot allocations it made.
// Callers that must reject an over-sell check the unfilled amount; lenient
// callers (imports) ignore it.
func DepleteLots(tx *gorm.DB, holdingID, accountID uuid.UUID, quantity, sellPrice float64, costBasisMethod string) (realized, unfilled float64, allocations []LotAllocation, err error) {
	// Sells may be recorded with a negative quantity (Fidelity's convention),
	// so work with the magnitude.
	if quantity < 0 {
		quantity = -quantity
	}

	dateOrder, idOrder := "purchase_date ASC", "id ASC"
	if strings.EqualFold(costBasisMethod, "LIFO") {
		dateOrder, idOrder = "purchase_date DESC", "id DESC"
	}

	var lots []models.TaxLot
	if err := tx.Where("holding_id = ? AND account_id = ? AND remaining_quantity > 0", holdingID, accountID).
		Order(dateOrder).Order(idOrder).Find(&lots).Error; err != nil {
		return 0, 0, nil, err
	}

	remaining := quantity
	for i := range lots {
		if remaining <= 0 {
			break
		}
		take := lots[i].RemainingQuantity
		if take > remaining {
			take = remaining
		}
		realized += (sellPrice - lots[i].PurchasePrice) * take
		lots[i].RemainingQuantity -= take
		remaining -= take
		allocations = append(allocations, LotAllocation{TaxLotID: lots[i].ID, Quantity: take})
		if err := tx.Save(&lots[i]).Error; err != nil {
			return 0, 0, nil, err
		}
	}
	return realized, remaining, allocations, nil
}

// RecalcHolding recomputes a holding's aggregates and saves them: position
// (quantity, cost basis, current value, unrealized gain) from its open tax lots,
// and realized gain from the sells recorded against it. Dividend income isn't
// derived here, so it's left untouched.
func RecalcHolding(tx *gorm.DB, holding *models.Holding) error {
	var lots []models.TaxLot
	if err := tx.Where("holding_id = ? AND remaining_quantity > 0", holding.ID).Find(&lots).Error; err != nil {
		return err
	}

	var quantity, costBasisTotal float64
	for _, lot := range lots {
		quantity += lot.RemainingQuantity
		costBasisTotal += lot.RemainingQuantity * lot.PurchasePrice
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

	// Realized gains come from sells recorded against this holding. Each sell's
	// amount is the proceeds (qty x price) and realized_gains is the gain, so
	// the cost basis of the shares sold is proceeds minus the gain. Realized
	// percent is the total gain over that cost basis.
	var sells []models.Transaction
	if err := tx.Where("holding_id = ? AND LOWER(action) = ?", holding.ID, "sell").Find(&sells).Error; err != nil {
		return err
	}
	var realizedAmount, realizedCostBasis float64
	for _, sell := range sells {
		realizedAmount += sell.RealizedGains
		realizedCostBasis += sell.Amount - sell.RealizedGains
	}
	holding.GainRealizedAmount = realizedAmount
	if realizedCostBasis > 0 {
		holding.GainRealizedPercent = (realizedAmount / realizedCostBasis) * 100
	} else {
		holding.GainRealizedPercent = 0
	}

	return tx.Save(holding).Error
}
