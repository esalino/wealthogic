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
	"github.com/eriksalino/wealthogic/api/internal/tax"
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
	actionBuy      = "Buy"
	actionSell     = "Sell"
	actionDividend = "Dividend"
)

// These match assetTypeFor's classification for treasury bills/notes and ETFs,
// both of which form tax lots like plain stocks.
const (
	treasuryAssetType = "Treasury"
	etfAssetType      = "ETF"
)

// mapAction normalizes a raw Fidelity action (e.g. "YOU BOUGHT ...") into the
// app's action vocabulary. A treasury maturing ("REDEMPTION PAYOUT ...") is a
// disposal, so it maps to a sell. Anything not mapped yet passes through raw.
func mapAction(rawAction string) string {
	upper := strings.ToUpper(rawAction)
	switch {
	case strings.HasPrefix(upper, "YOU BOUGHT"):
		return actionBuy
	case strings.HasPrefix(upper, "YOU SOLD"), strings.HasPrefix(upper, "REDEMPTION PAYOUT"):
		return actionSell
	case strings.HasPrefix(upper, "DIVIDEND"):
		// A cash dividend received. Reinvestments ("REINVESTMENT ...") aren't
		// mapped yet - they also open a lot, so they stay pass-through for now.
		return actionDividend
	default:
		return rawAction
	}
}

// isLotForming reports whether an asset type is tracked as tax lots. Stocks,
// ETFs, and treasuries are - they realize capital gains on sale; options and
// cash aren't yet.
func isLotForming(assetType *string) bool {
	return assetType != nil &&
		(*assetType == defaultAssetType || *assetType == etfAssetType || *assetType == treasuryAssetType)
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

		quantity := parseDollarPtr(record[txnColQuantity])
		price := parseDollarPtr(record[txnColPrice])
		amount := parseDollar(record[txnColAmount])

		// Treasuries quote price per $100 of face value while quantity is the
		// face value, so the raw price is off by scale. Derive the effective
		// per-unit price from the actual amount instead - exact, and treasuries
		// carry no commission/fees.
		if assetType != nil && *assetType == treasuryAssetType && quantity != nil && *quantity != 0 && amount != 0 {
			perUnit := amount / *quantity
			if perUnit < 0 {
				perUnit = -perUnit
			}
			price = &perUnit
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
				Quantity:         quantity,
				Price:            price,
				Amount:           amount,
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

		// One applier for the whole file: every realized event in it is
		// evaluated against the same rules, so they're loaded once.
		applier, err := tax.NewApplier(tx)
		if err != nil {
			return fmt.Errorf("failed to load tax rules: %w", err)
		}

		// Insert oldest first so the PK (UUIDv7) order is chronological.
		for i := len(rows) - 1; i >= 0; i-- {
			txn := &rows[i].txn

			// Dividends (and later interest) are income, not trades - route them
			// to the distribution ledger and keep them out of transactions and
			// lot math entirely. Symbol/asset type may be unset (e.g. a dividend
			// row where the source left the symbol column blank); the row is
			// still income, so it belongs here regardless.
			if txn.Action == actionDividend {
				dist := models.Distribution{
					Category:    "dividend",
					AccountID:   opts.AccountID,
					Symbol:      txn.Symbol,
					AssetType:   txn.AssetType,
					PaymentDate: txn.Date,
					Amount:      txn.Amount,
				}
				// Link to an existing holding if there is one; a dividend never
				// creates a holding on its own.
				var holding *models.Holding
				var found models.Holding
				switch err := tx.Where("symbol = ?", txn.Symbol).First(&found).Error; {
				case err == nil:
					dist.HoldingID = &found.ID
					holding = &found
				case !errors.Is(err, gorm.ErrRecordNotFound):
					return fmt.Errorf("failed to look up holding %s: %w", txn.Symbol, err)
				}
				if err := tx.Create(&dist).Error; err != nil {
					return fmt.Errorf("failed to create distribution for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
				}
				// The income is received now, so its tax treatment is settled
				// now - the paying security's tax class decides whether it's
				// ordinary, qualified-eligible, or exempt in each jurisdiction.
				if err := applier.ApplyToDistribution(&dist, holding); err != nil {
					return fmt.Errorf("failed to apply tax treatment for %s: %w", txn.Symbol, err)
				}
				result.Created++

				uploadTxn := models.UploadTransaction{
					AssetType:        txn.AssetType,
					Symbol:           txn.Symbol,
					AssetDescription: txn.AssetDescription,
					Action:           rows[i].rawAction,
					Date:             txn.Date,
					Amount:           txn.Amount,
					DistributionID:   &dist.ID,
					UploadID:         upload.ID,
				}
				if err := tx.Create(&uploadTxn).Error; err != nil {
					return fmt.Errorf("failed to create upload transaction for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
				}
				continue
			}

			// A lot-forming buy (stock or treasury) IS a tax lot (seed its
			// remaining quantity); the matching sell/redemption depletes open
			// lots (FIFO/LIFO per the account) and realizes gains. Options and
			// other non-lot actions do neither.
			isBuy := txn.Action == actionBuy && isLotForming(txn.AssetType) && txn.Quantity != nil && txn.Price != nil
			isSell := txn.Action == actionSell && isLotForming(txn.AssetType) && txn.Quantity != nil && txn.Price != nil

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
				q := *txn.Quantity
				txn.RemainingQuantity = &q
				affected[holding.ID] = true
			}

			if isSell {
				// If we have no holding for the symbol (its buys predate this
				// file), record the sell without depleting.
				err := tx.Where("symbol = ?", txn.Symbol).First(&holding).Error
				switch {
				case errors.Is(err, gorm.ErrRecordNotFound):
					isSell = false // nothing to sell against; just record it
				case err != nil:
					return fmt.Errorf("failed to look up holding %s: %w", txn.Symbol, err)
				default:
					txn.HoldingID = &holding.ID
					affected[holding.ID] = true
				}
			}

			if err := tx.Create(txn).Error; err != nil {
				return fmt.Errorf("failed to create transaction for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
			}
			result.Created++

			// Deplete lots for the sell now that the transaction exists (Gain
			// rows link to it). Imports are lenient: an unfillable sell realizes
			// only what it could match.
			if isSell {
				realized, _, err := portfolio.DepleteLots(tx, txn, account.DefaultCostBasis, applier)
				if err != nil {
					return fmt.Errorf("failed to deplete lots for %s: %w", txn.Symbol, err)
				}
				txn.RealizedGains = realized
				if err := tx.Model(txn).Update("realized_gains", realized).Error; err != nil {
					return fmt.Errorf("failed to set realized gains for %s: %w", txn.Symbol, err)
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
				TransactionID:    &txn.ID,
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
