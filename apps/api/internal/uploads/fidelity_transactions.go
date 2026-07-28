package uploads

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Column indices in a Fidelity "Accounts_History" (transactions) export.
const (
	txnColRunDate        = 0
	txnColAction         = 1
	txnColSymbol         = 2
	txnColDescription    = 3
	txnColPrice          = 8
	txnColQuantity       = 9
	txnColCommission     = 11
	txnColFees           = 12
	txnColAmount         = 14
	txnColSettlementDate = 16
	txnMinColumns        = 17
)

const fidelityDateLayout = "01/02/2006"

// buyActionPrefix identifies a Fidelity "buy" action, e.g. "YOU BOUGHT ...".
const buyActionPrefix = "YOU BOUGHT"

// isStockBuy reports whether a transaction represents an outright stock
// purchase, as opposed to an option trade, ETF, treasury, or other non-stock
// action that also happens to start with "YOU BOUGHT".
func isStockBuy(action string, assetType *string) bool {
	return assetType != nil && *assetType == defaultAssetType && strings.HasPrefix(strings.ToUpper(action), buyActionPrefix)
}

// fidelityTransactionsHandler parses a Fidelity account history CSV and
// creates one Transaction per row, tied to the account passed in Options.
// Fidelity lists rows newest-first, but we want insertion order (and thus PK
// order) to match chronological order, so rows are buffered and inserted in
// reverse.
// Positions are not touched yet - each upload just appends transactions, so
// re-uploading the same file will duplicate rows until we add dedup logic.
type fidelityTransactionsHandler struct{}

func (h *fidelityTransactionsHandler) Process(db *gorm.DB, file io.Reader, opts Options) (*Result, error) {
	if opts.AccountID == uuid.Nil {
		return nil, fmt.Errorf("account_id is required for transaction uploads")
	}

	var account models.Account
	if err := db.First(&account, "id = ?", opts.AccountID).Error; err != nil {
		return nil, fmt.Errorf("account %s not found", opts.AccountID)
	}

	reader := csv.NewReader(file)
	// The export has leading blank lines and a trailing disclaimer/footer
	// with varying column counts, so don't enforce a fixed number of fields.
	reader.FieldsPerRecord = -1

	result := &Result{}
	var transactions []models.Transaction
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse csv: %w", err)
		}

		if len(record) < txnMinColumns {
			result.Skipped++
			continue
		}

		runDate := strings.TrimSpace(record[txnColRunDate])
		action := strings.TrimSpace(record[txnColAction])
		if runDate == "" || runDate == "Run Date" || action == "" {
			result.Skipped++
			continue
		}

		date, err := time.Parse(fidelityDateLayout, runDate)
		if err != nil {
			result.Skipped++
			continue
		}

		symbol := strings.TrimSuffix(strings.TrimSpace(record[txnColSymbol]), "**")
		description := strings.TrimSpace(record[txnColDescription])

		// Cash movements (transfers, EFTs) have no underlying security, so
		// there's no description to derive an asset type from - leave both
		// unset rather than guessing.
		var assetType, assetDescription *string
		if symbol != "" && description != "" && description != "No Description" {
			t := assetTypeFor(description)
			assetType = &t
			assetDescription = &description
		}

		transactions = append(transactions, models.Transaction{
			AccountID:        opts.AccountID,
			AssetType:        assetType,
			Symbol:           symbol,
			AssetDescription: assetDescription,
			Action:           action,
			Date:             date,
			Quantity:         parseDollarPtr(record[txnColQuantity]),
			Price:            parseDollarPtr(record[txnColPrice]),
			Amount:           parseDollar(record[txnColAmount]),
			Commission:       parseDollar(record[txnColCommission]),
			Fees:             parseDollar(record[txnColFees]),
			SettlementDate:   parseDatePtr(record[txnColSettlementDate]),
		})
	}

	// Fidelity lists newest first; insert oldest first so the auto-increment
	// PK order matches chronological order. The whole batch is wrapped in a
	// transaction so a bad row doesn't leave a partially imported file.
	err := db.Transaction(func(tx *gorm.DB) error {
		for i := len(transactions) - 1; i >= 0; i-- {
			txn := &transactions[i]
			if err := tx.Create(txn).Error; err != nil {
				return fmt.Errorf("failed to create transaction for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
			}
			result.Created++

			// A stock buy opens a new tax lot at the transaction's price and
			// quantity. Sells, options, and other non-stock actions don't -
			// that's a later step.
			if isStockBuy(txn.Action, txn.AssetType) && txn.Quantity != nil && txn.Price != nil {
				lot := models.TaxLot{
					AssetType:         *txn.AssetType,
					Symbol:            txn.Symbol,
					AssetDescription:  *txn.AssetDescription,
					PurchaseDate:      txn.Date,
					PurchaseQuantity:  *txn.Quantity,
					PurchasePrice:     *txn.Price,
					RemainingQuantity: *txn.Quantity,
					AccountID:         txn.AccountID,
				}

				// Transactions and holdings come from separate Fidelity
				// exports, so the holding may not exist yet - link it if we
				// find one, otherwise leave it unset.
				var holding models.Holding
				if err := tx.Where("symbol = ?", txn.Symbol).First(&holding).Error; err == nil {
					lot.HoldingID = &holding.ID
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("failed to look up holding %s: %w", txn.Symbol, err)
				}

				if err := tx.Create(&lot).Error; err != nil {
					return fmt.Errorf("failed to create tax lot for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// parseDatePtr parses a Fidelity date column, returning nil for blank values
// or sentinels like "Processing" instead of a zero time.Time.
func parseDatePtr(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(fidelityDateLayout, s)
	if err != nil {
		return nil
	}
	return &t
}
