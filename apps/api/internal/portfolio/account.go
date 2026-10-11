package portfolio

import (
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SetAccountCash records the cash an import reported for an account, unless the
// account already holds a figure from a later date. Files arrive out of order -
// last year's history uploaded after this month's positions - and an older
// snapshot must not overwrite a newer one.
func SetAccountCash(tx *gorm.DB, accountID uuid.UUID, cash float64, asOf time.Time) error {
	var account models.Account
	if err := tx.Select("id", "cash_balance_as_of").First(&account, "id = ?", accountID).Error; err != nil {
		return err
	}
	if account.CashBalanceAsOf != nil && account.CashBalanceAsOf.After(asOf) {
		return nil
	}
	return tx.Model(&models.Account{}).Where("id = ?", accountID).
		Updates(map[string]any{"cash_balance": cash, "cash_balance_as_of": asOf}).Error
}

// RecalcAccountBalance derives an account's balance: its cash plus what its
// open lots are worth at each holding's last price. A written option is a
// liability, so a short lot counts against the balance.
//
// Holdings are shared across accounts, so the position is read from the
// account's own lots rather than from a holding's aggregate CurrentValue.
//
// An account with no lots and no reported cash has nothing to derive a balance
// from - a bank account with no importer - so its hand-entered balance stands.
func RecalcAccountBalance(tx *gorm.DB, accountID uuid.UUID) error {
	var account models.Account
	if err := tx.Select("id", "cash_balance", "cash_balance_as_of").First(&account, "id = ?", accountID).Error; err != nil {
		return err
	}

	var lotCount int64
	if err := tx.Model(&models.TaxLot{}).Where("account_id = ?", accountID).Count(&lotCount).Error; err != nil {
		return err
	}
	if lotCount == 0 && account.CashBalanceAsOf == nil {
		return nil
	}

	var lots []models.TaxLot
	if err := tx.Where("account_id = ? AND remaining_quantity > ?", accountID, quantityEpsilon).
		Find(&lots).Error; err != nil {
		return err
	}

	holdingIDs := make([]uuid.UUID, 0, len(lots))
	seen := map[uuid.UUID]bool{}
	for _, lot := range lots {
		if !seen[lot.HoldingID] {
			seen[lot.HoldingID] = true
			holdingIDs = append(holdingIDs, lot.HoldingID)
		}
	}
	holdings := map[uuid.UUID]models.Holding{}
	if len(holdingIDs) > 0 {
		var found []models.Holding
		if err := tx.Find(&found, "id IN ?", holdingIDs).Error; err != nil {
			return err
		}
		for _, h := range found {
			holdings[h.ID] = h
		}
	}

	balance := account.CashBalance
	for _, lot := range lots {
		h, ok := holdings[lot.HoldingID]
		if !ok {
			continue
		}
		value := lot.RemainingQuantity * h.LastPrice * h.Multiplier()
		if lot.Direction == models.DirectionShort {
			balance -= value
			continue
		}
		balance += value
	}

	return tx.Model(&models.Account{}).Where("id = ?", accountID).Update("balance", balance).Error
}

// RecalcAllAccountBalances re-derives every account's balance. A price moves
// every account holding the symbol, so a reprice can't tell which accounts it
// touched without asking each one anyway.
func RecalcAllAccountBalances(tx *gorm.DB) error {
	var ids []uuid.UUID
	if err := tx.Model(&models.Account{}).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := RecalcAccountBalance(tx, id); err != nil {
			return err
		}
	}
	return nil
}
