package uploads

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
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

// The app's normalized action vocabulary. Only buys and sells are mapped for
// now; any other raw action passes through unchanged.
const (
	actionBuy  = "Buy"
	actionSell = "Sell"
)

// mapAction normalizes a raw Fidelity action (e.g. "YOU BOUGHT ...") into the
// app's action vocabulary. Anything not mapped yet passes through raw.
func mapAction(rawAction string) string {
	upper := strings.ToUpper(rawAction)
	switch {
	case strings.HasPrefix(upper, "YOU BOUGHT"):
		return actionBuy
	case strings.HasPrefix(upper, "YOU SOLD"):
		return actionSell
	default:
		return rawAction
	}
}

// isStockBuy reports whether a mapped transaction is an outright stock purchase
// (as opposed to an ETF, option, treasury, or other non-stock buy). Only these
// open a tax lot.
func isStockBuy(mappedAction string, assetType *string) bool {
	return mappedAction == actionBuy && assetType != nil && *assetType == defaultAssetType
}

// isStockSell mirrors isStockBuy for outright stock sales, which deplete lots.
func isStockSell(mappedAction string, assetType *string) bool {
	return mappedAction == actionSell && assetType != nil && *assetType == defaultAssetType
}

// parsedRow pairs a parsed Transaction (with its mapped action) with the raw
// action from the file, which the UploadTransaction preserves.
type parsedRow struct {
	txn       models.Transaction
	rawAction string
}

// fidelityTransactionsHandler parses a Fidelity account history CSV. For each
// row it creates a Transaction (with a mapped action) plus, as a record of the
// import, an UploadTransaction (with the raw action) linked back to it; all of
// them tie to an Upload row logging the file. A stock buy also opens a tax lot.
//
// Fidelity lists rows newest-first, so rows are buffered and inserted in reverse
// to keep insertion (and thus PK) order chronological.
// Positions aren't deduped yet - re-uploading the same file duplicates rows.
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
	// The export has leading blank lines and a trailing disclaimer/footer with
	// varying column counts, so don't enforce a fixed number of fields.
	reader.FieldsPerRecord = -1

	result := &Result{}
	var rows []parsedRow
	var minDate, maxDate time.Time
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
		rawAction := strings.TrimSpace(record[txnColAction])
		if runDate == "" || runDate == "Run Date" || rawAction == "" {
			result.Skipped++
			continue
		}

		date, err := time.Parse(fidelityDateLayout, runDate)
		if err != nil {
			result.Skipped++
			continue
		}

		if minDate.IsZero() || date.Before(minDate) {
			minDate = date
		}
		if maxDate.IsZero() || date.After(maxDate) {
			maxDate = date
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

		rows = append(rows, parsedRow{
			rawAction: rawAction,
			txn: models.Transaction{
				AccountID:        opts.AccountID,
				AssetType:        assetType,
				Symbol:           symbol,
				AssetDescription: assetDescription,
				Action:           mapAction(rawAction),
				Date:             date,
				Quantity:         parseDollarPtr(record[txnColQuantity]),
				Price:            parseDollarPtr(record[txnColPrice]),
				Amount:           parseDollar(record[txnColAmount]),
				Commission:       parseDollar(record[txnColCommission]),
				Fees:             parseDollar(record[txnColFees]),
				SettlementDate:   parseDatePtr(record[txnColSettlementDate]),
			},
		})
	}

	// Date range covered by the file (nil if nothing parsed).
	var startDate, endDate *time.Time
	if !minDate.IsZero() {
		startDate = &minDate
		endDate = &maxDate
	}

	// The whole batch is wrapped in a transaction so a bad row doesn't leave a
	// partially imported file.
	err := db.Transaction(func(tx *gorm.DB) error {
		// Log the import itself; every row's UploadTransaction links to it.
		upload := models.Upload{
			FileName:  opts.FileName,
			AccountID: opts.AccountID,
			StartDate: startDate,
			EndDate:   endDate,
		}
		if err := tx.Create(&upload).Error; err != nil {
			return fmt.Errorf("failed to create upload: %w", err)
		}

		// Holdings that got new lots and need their aggregates recomputed.
		affected := map[uuid.UUID]bool{}

		// Insert oldest first so the PK (UUIDv7) order is chronological.
		for i := len(rows) - 1; i >= 0; i-- {
			txn := &rows[i].txn

			// A stock buy opens a new tax lot; a stock sell depletes open lots
			// (FIFO/LIFO per the account) and realizes gains. Options and other
			// non-stock actions do neither.
			isBuy := isStockBuy(txn.Action, txn.AssetType) && txn.Quantity != nil && txn.Price != nil
			isSell := isStockSell(txn.Action, txn.AssetType) && txn.Quantity != nil && txn.Price != nil

			var holding models.Holding
			if isBuy {
				// Transactions and holdings come from separate Fidelity
				// exports, so the holding may not exist yet. Link an existing
				// one, otherwise create a bare holding from what the
				// transaction knows - last price and current value stay zero
				// until a holdings import fills them in.
				err := tx.Where("symbol = ?", txn.Symbol).First(&holding).Error
				switch {
				case errors.Is(err, gorm.ErrRecordNotFound):
					holding = models.Holding{
						AssetType:   *txn.AssetType,
						Symbol:      txn.Symbol,
						Description: *txn.AssetDescription,
					}
					if err := tx.Create(&holding).Error; err != nil {
						return fmt.Errorf("failed to create holding %s: %w", txn.Symbol, err)
					}
				case err != nil:
					return fmt.Errorf("failed to look up holding %s: %w", txn.Symbol, err)
				}
				txn.HoldingID = &holding.ID
				affected[holding.ID] = true
			}

			if isSell {
				// Deplete the holding's open lots for this account. If we have no
				// holding for the symbol (its buys predate this file), record the
				// sell as-is without depleting. Imports are lenient: an
				// unfillable sell realizes only what it could match.
				err := tx.Where("symbol = ?", txn.Symbol).First(&holding).Error
				switch {
				case errors.Is(err, gorm.ErrRecordNotFound):
					// nothing to sell against; just record the transaction
				case err != nil:
					return fmt.Errorf("failed to look up holding %s: %w", txn.Symbol, err)
				default:
					txn.HoldingID = &holding.ID
					affected[holding.ID] = true
					realized, _, err := portfolio.DepleteLots(tx, holding.ID, opts.AccountID, *txn.Quantity, *txn.Price, account.DefaultCostBasis)
					if err != nil {
						return fmt.Errorf("failed to deplete lots for %s: %w", txn.Symbol, err)
					}
					txn.RealizedGains = realized
				}
			}

			if err := tx.Create(txn).Error; err != nil {
				return fmt.Errorf("failed to create transaction for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
			}
			result.Created++

			if isBuy {
				lot := models.TaxLot{
					AssetType:         *txn.AssetType,
					Symbol:            txn.Symbol,
					AssetDescription:  *txn.AssetDescription,
					PurchaseDate:      txn.Date,
					PurchaseQuantity:  *txn.Quantity,
					PurchasePrice:     *txn.Price,
					RemainingQuantity: *txn.Quantity,
					HoldingID:         &holding.ID,
					AccountID:         txn.AccountID,
				}
				if err := tx.Create(&lot).Error; err != nil {
					return fmt.Errorf("failed to create tax lot for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
				}
			}

			// Record the raw import row, keyed to the upload and the
			// transaction it produced.
			uploadTxn := models.UploadTransaction{
				AssetType:        txn.AssetType,
				Symbol:           txn.Symbol,
				AssetDescription: txn.AssetDescription,
				Action:           rows[i].rawAction,
				Date:             txn.Date,
				Quantity:         txn.Quantity,
				Price:            txn.Price,
				Amount:           txn.Amount,
				Commission:       txn.Commission,
				Fees:             txn.Fees,
				SettlementDate:   txn.SettlementDate,
				RealizedGains:    txn.RealizedGains,
				TransactionID:    txn.ID,
				UploadID:         upload.ID,
			}
			if err := tx.Create(&uploadTxn).Error; err != nil {
				return fmt.Errorf("failed to create upload transaction for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
			}
		}

		// Recompute each holding that got new lots so its quantity, cost basis,
		// and value reflect the imported buys.
		for holdingID := range affected {
			var holding models.Holding
			if err := tx.First(&holding, "id = ?", holdingID).Error; err != nil {
				return fmt.Errorf("failed to load holding %s for recompute: %w", holdingID, err)
			}
			if err := portfolio.RecalcHolding(tx, &holding); err != nil {
				return fmt.Errorf("failed to recompute holding %s: %w", holding.Symbol, err)
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
