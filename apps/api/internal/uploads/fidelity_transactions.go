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
	actionExpired  = "Expired"
)

// Lot effects and directions, mirroring models.Transaction.
const (
	effectOpen     = "open"
	effectClose    = "close"
	directionLong  = "long"
	directionShort = "short"
)

// These match assetTypeFor's classification for treasury bills/notes and ETFs,
// both of which form tax lots like plain stocks.
const (
	treasuryAssetType = "Treasury"
	etfAssetType      = "ETF"
	optionAssetType   = "Option"
)

// mapAction normalizes a raw Fidelity action into the app's action vocabulary,
// and says what it does to the lot ledger.
//
// The lot effect can't be read off the action alone once options are involved:
// writing an option OPENS a position by selling it, and buying it back CLOSES
// one. Fidelity spells this out ("YOU SOLD OPENING TRANSACTION"), so the effect
// comes from the OPENING/CLOSING marker and the direction from which side of the
// trade opened the position. A stock buy is an open/long and a stock sell a
// close/long - the ordinary case of the same rule.
//
// quantity disambiguates an expiration, which names no side. Fidelity records it
// as the offsetting trade that removes the position, signed like any close: a
// long position (+2) is closed by -2, a written one (-2) by +2. So a negative
// quantity closes a long and a positive one closes a short.
// Anything not mapped yet passes through raw with no lot effect.
func mapAction(rawAction string, quantity *float64) (action, effect, direction string) {
	upper := strings.ToUpper(rawAction)
	opening := strings.Contains(upper, "OPENING TRANSACTION")
	closing := strings.Contains(upper, "CLOSING TRANSACTION")

	switch {
	case strings.HasPrefix(upper, "EXPIRED"):
		// Expiring closes the position at zero. A long contract expires
		// worthless (full loss of premium); a written one expires to the
		// writer's benefit (the whole premium is gain).
		dir := directionLong
		if quantity != nil && *quantity > 0 {
			dir = directionShort
		}
		return actionExpired, effectClose, dir

	case strings.HasPrefix(upper, "YOU BOUGHT"):
		switch {
		case closing:
			// Buying to close is covering a written option.
			return actionBuy, effectClose, directionShort
		case opening:
			return actionBuy, effectOpen, directionLong
		}
		return actionBuy, effectOpen, directionLong

	case strings.HasPrefix(upper, "YOU SOLD"), strings.HasPrefix(upper, "REDEMPTION PAYOUT"):
		if opening {
			// Selling to open is writing an option: premium received now, the
			// cost of closing it comes later, or never.
			return actionSell, effectOpen, directionShort
		}
		// A plain sale, a sell-to-close, and a treasury redemption all dispose
		// of a long position.
		return actionSell, effectClose, directionLong

	case strings.HasPrefix(upper, "DIVIDEND"):
		// A cash dividend received. Reinvestments ("REINVESTMENT ...") aren't
		// mapped yet - they also open a lot, so they stay pass-through for now.
		return actionDividend, "", ""
	}
	return rawAction, "", ""
}

// isLotForming reports whether an asset type is tracked as tax lots. Stocks,
// ETFs, treasuries, and options are - they all realize capital gains when
// disposed of. Cash isn't.
func isLotForming(assetType *string) bool {
	return assetType != nil &&
		(*assetType == defaultAssetType || *assetType == etfAssetType ||
			*assetType == treasuryAssetType || *assetType == optionAssetType)
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
	// A positions export parses as transactions without error, so require this
	// file's own header before trusting a single row of it.
	var sawHeader bool
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse csv: %w", err)
		}

		if !sawHeader {
			if !matchesHeader(record, transactionsHeaderMarkers) {
				result.Skipped++
				continue
			}
			sawHeader = true
			continue
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

		action, effect, direction := mapAction(rawAction, quantity)

		rows = append(rows, parsedRow{
			rawAction: rawAction,
			txn: models.Transaction{
				AccountID:        opts.AccountID,
				AssetType:        assetType,
				Symbol:           symbol,
				AssetDescription: assetDescription,
				Action:           action,
				Effect:           effect,
				Direction:        direction,
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

	if !sawHeader {
		return nil, wrongFileError("transactions")
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

		// What the account already holds over this file's date range, so an
		// overlapping export adds only the rows that are actually new.
		existing, err := loadExistingRows(tx, opts.AccountID, startDate, endDate)
		if err != nil {
			return fmt.Errorf("failed to load existing rows: %w", err)
		}

		// Insert oldest first so the PK (UUIDv7) order is chronological.
		for i := len(rows) - 1; i >= 0; i-- {
			txn := &rows[i].txn

			// Already imported, from an earlier file covering the same dates.
			var sig signature
			if txn.Action == actionDividend {
				sig = distSignature(txn.Date, "dividend", txn.Symbol, txn.Amount)
			} else {
				sig = txnSignature(txn.Date, txn.Action, txn.Effect, txn.Direction, txn.Symbol, txn.Quantity, txn.Amount)
			}
			if existing.claim(sig) {
				result.Duplicates++
				continue
			}

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
				var found models.Holding
				switch err := tx.Where("symbol = ?", txn.Symbol).First(&found).Error; {
				case err == nil:
					dist.HoldingID = &found.ID
				case !errors.Is(err, gorm.ErrRecordNotFound):
					return fmt.Errorf("failed to look up holding %s: %w", txn.Symbol, err)
				}
				if err := tx.Create(&dist).Error; err != nil {
					return fmt.Errorf("failed to create distribution for %s on %s: %w", txn.Symbol, txn.Date.Format(fidelityDateLayout), err)
				}
				// Derive the realized event and its tax treatments from the
				// payment just recorded, exactly as a hand-entered distribution
				// will - the import is just another way the source record arrives.
				if err := portfolio.RealizeDistribution(tx, &dist, applier); err != nil {
					return fmt.Errorf("failed to realize distribution for %s: %w", txn.Symbol, err)
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

			// An opening trade IS a tax lot (seed its remaining quantity); a
			// closing trade depletes open lots on the same side (FIFO/LIFO per
			// the account) and realizes gains. Non-lot actions do neither.
			//
			// A close needs no price: an expiration has none, and closing at
			// zero is exactly what expiring means.
			isOpen := txn.Effect == effectOpen && isLotForming(txn.AssetType) && txn.Quantity != nil && txn.Price != nil
			isClose := txn.Effect == effectClose && isLotForming(txn.AssetType) && txn.Quantity != nil

			var holding models.Holding
			if isOpen {
				// Transactions and holdings come from separate Fidelity
				// exports, so the holding may not exist yet. Link an existing
				// one, otherwise create a bare holding from what the
				// transaction knows - last price and current value stay zero
				// until a holdings import fills them in.
				err := tx.Where("symbol = ?", txn.Symbol).First(&holding).Error
				switch {
				case errors.Is(err, gorm.ErrRecordNotFound):
					holding = models.Holding{
						AssetType:          *txn.AssetType,
						Symbol:             txn.Symbol,
						Description:        *txn.AssetDescription,
						ContractMultiplier: 1,
					}
					applyOptionDetail(&holding)
					if err := tx.Create(&holding).Error; err != nil {
						return fmt.Errorf("failed to create holding %s: %w", txn.Symbol, err)
					}
				case err != nil:
					return fmt.Errorf("failed to look up holding %s: %w", txn.Symbol, err)
				}

				// A treasury has a quantity and a per-unit price like any other
				// position, so it can be valued - but only if it has a price at
				// all, and a holding created from a transaction has none until a
				// positions file covers it. Seed it from the trade.
				//
				// Only for treasuries, and only as a fallback: a trade price is a
				// historical fact, which for a bill converging to par is close
				// enough to value the position, and for a volatile equity is not.
				// A positions import always wins, so this never overrides one.
				if *txn.AssetType == treasuryAssetType && holding.LastPrice == 0 {
					if err := tx.Model(&holding).Update("last_price", *txn.Price).Error; err != nil {
						return fmt.Errorf("failed to seed last price for %s: %w", txn.Symbol, err)
					}
					holding.LastPrice = *txn.Price
				}

				txn.HoldingID = &holding.ID
				// Fidelity signs quantity by trade direction, so writing an
				// option arrives negative; the lot's size is its magnitude and
				// Direction carries the side.
				q := *txn.Quantity
				if q < 0 {
					q = -q
				}
				txn.RemainingQuantity = &q
				affected[holding.ID] = true
			}

			if isClose {
				// If we have no holding for the symbol (its opens predate this
				// file), record the trade without depleting.
				err := tx.Where("symbol = ?", txn.Symbol).First(&holding).Error
				switch {
				case errors.Is(err, gorm.ErrRecordNotFound):
					isClose = false // nothing to close against; just record it
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

			// Deplete lots now that the transaction exists (realized events link
			// to it). Imports are lenient: an unfillable close realizes only
			// what it could match.
			if isClose {
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

// applyOptionDetail decodes an option contract's symbol onto the holding, so a
// position can be grouped by underlying and sorted by expiry rather than being
// an opaque ticker. It also sets the contract multiplier, which every basis and
// proceeds figure for the holding scales by. A non-option symbol is left alone.
func applyOptionDetail(holding *models.Holding) {
	detail, ok := models.ParseOptionSymbol(holding.Symbol)
	if !ok {
		return
	}
	expiration := detail.Expiration
	strike := detail.Strike

	holding.AssetType = models.AssetTypeOption
	holding.Underlying = detail.Underlying
	holding.OptionType = detail.Type
	holding.StrikePrice = &strike
	holding.ExpirationDate = &expiration
	holding.ContractMultiplier = models.OptionContractMultiplier
}
