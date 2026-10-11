package uploads

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Column indices in a Fidelity "Portfolio Positions" export.
const (
	fidColAccount      = 0
	fidColSymbol       = 2
	fidColDescription  = 3
	fidColQuantity     = 4
	fidColLastPrice    = 5
	fidColCurrentValue = 7
	fidMinColumns      = 16
)

// optionWordRegex matches PUT/CALL as whole words so it doesn't misfire on
// unrelated substrings, and matches whether the word leads the description
// (as in transaction rows, e.g. "PUT (MAR) MARRIOTT...") or trails it (as in
// holding rows, e.g. "MAR JUL 17 2026 $320 PUT").
var optionWordRegex = regexp.MustCompile(`\b(PUT|CALL)\b`)

// downloadedRegex finds the snapshot date in the export's footer line, e.g.
// "Date downloaded Oct-03-2026 5:14 p.m ET".
var downloadedRegex = regexp.MustCompile(`Date downloaded ([A-Za-z]{3}-\d{2}-\d{4})`)

// pendingActivitySymbol is the row Fidelity adds for cash movements that
// haven't settled. It has a value but no description and no security behind it.
const pendingActivitySymbol = "PENDING ACTIVITY"

const (
	defaultAssetType = "Stock"
	// cashAssetType is the asset type for money-market-style holdings, whose
	// value we take directly from the CSV's Current Value column rather than
	// deriving it from quantity * last price.
	cashAssetType = "Money Market"
)

// assetTypeFor maps a substring of the Fidelity description to an asset type.
func assetTypeFor(description string) string {
	desc := strings.ToUpper(description)
	switch {
	case strings.Contains(desc, "MONEY MARKET"):
		return cashAssetType
	case strings.Contains(desc, "TREAS"):
		return "Treasury"
	case optionWordRegex.MatchString(desc):
		return "Option"
	case strings.Contains(desc, "ETF"):
		return "ETF"
	}
	return defaultAssetType
}

// fidelityHoldingsHandler parses a Fidelity portfolio positions CSV and
// upserts one Holding per symbol.
type fidelityHoldingsHandler struct{}

func (h *fidelityHoldingsHandler) Process(db *gorm.DB, file io.Reader, opts Options) (*Result, error) {
	reader := csv.NewReader(file)
	// The export has a trailing disclaimer and footer rows with varying
	// column counts, so don't enforce a fixed number of fields.
	reader.FieldsPerRecord = -1

	result := &Result{}
	// A transactions export parses as holdings without error (same Symbol and
	// Description columns), so require this file's own header before trusting a
	// single row of it.
	var sawHeader bool

	// The account's cash: its core money market plus pending activity. The
	// file is a snapshot of the day it was downloaded, which its footer says;
	// without one, the upload itself is the best guess at when.
	var cash float64
	asOf := time.Now()
	accountNumbers := map[string]bool{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse csv: %w", err)
		}

		if !sawHeader {
			if !matchesHeader(record, holdingsHeaderMarkers) {
				result.Skipped++
				continue
			}
			sawHeader = true
			continue
		}

		// Both are shorter than a position row, so they're picked out before
		// the length check below throws them away.
		if d, ok := downloadedDate(record); ok {
			asOf = d
			result.Skipped++
			continue
		}
		if value, ok := pendingActivity(record); ok {
			accountNumbers[strings.TrimSpace(record[fidColAccount])] = true
			cash += value
			result.Skipped++
			continue
		}

		if len(record) < fidMinColumns {
			result.Skipped++
			continue
		}

		if acct := strings.TrimSpace(record[fidColAccount]); acct != "" {
			accountNumbers[acct] = true
		}

		symbol := strings.TrimSuffix(strings.TrimSpace(record[fidColSymbol]), "**")
		description := strings.TrimSpace(record[fidColDescription])
		if symbol == "" || symbol == "Symbol" || description == "" {
			result.Skipped++
			continue
		}

		assetType := assetTypeFor(description)
		lastPrice := parseDollar(record[fidColLastPrice])

		// Treasuries are quoted per $100 of face value while quantity is the
		// face value itself, so the raw price is off by a factor of 100 against
		// every other holding. Rescale to a per-unit price here, matching what
		// the transaction importer derives, or quantity * price overstates the
		// position a hundredfold.
		if assetType == treasuryAssetType {
			lastPrice /= 100
		}

		// A row with no quantity has no share count a position could be derived
		// from and never forms a tax lot - a money-market or cash balance. Its
		// value can only come from the broker, so take it.
		//
		// Everything else deliberately takes nothing but its identity and price
		// from this file: quantity, cost basis, and value come from the
		// transaction ledger, which is the one place a position is actually
		// derived. A holding left sitting at zero here is the signal that its
		// transactions haven't been imported yet.
		valueOnly := parseDollar(record[fidColQuantity]) == 0
		currentValue := 0.0
		if valueOnly {
			currentValue = parseDollar(record[fidColCurrentValue])
			cash += currentValue
		}

		var holding models.Holding
		err = db.Where("symbol = ?", symbol).First(&holding).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			holding = models.Holding{
				AssetType:          assetType,
				Symbol:             symbol,
				Description:        description,
				LastPrice:          lastPrice,
				CurrentValue:       currentValue,
				ContractMultiplier: 1,
			}
			applyOptionDetail(&holding)
			if err := db.Create(&holding).Error; err != nil {
				return nil, fmt.Errorf("failed to create holding %s: %w", symbol, err)
			}
			result.Created++
			opts.Enricher.EnrichQuietly(context.Background(), db, &holding)
		case err != nil:
			return nil, fmt.Errorf("failed to look up holding %s: %w", symbol, err)
		default:
			holding.AssetType = assetType
			holding.Description = description
			holding.LastPrice = lastPrice
			applyOptionDetail(&holding)
			// Only a value-only holding's value comes from here; for anything
			// else the transaction ledger owns it and must not be clobbered by
			// a re-upload of the positions file.
			if valueOnly {
				holding.CurrentValue = currentValue
			}
			if err := db.Save(&holding).Error; err != nil {
				return nil, fmt.Errorf("failed to update holding %s: %w", symbol, err)
			}
			result.Updated++
		}

		// A new price changes what the position is worth, and this file supplies
		// the price without supplying the value - so the value has to be
		// re-derived from the lots here, or it stays stale at whatever the last
		// transaction import left. Skipped for value-only holdings, which have
		// no lots and whose value came straight from the file above.
		if !valueOnly {
			if err := portfolio.RecalcHolding(db, &holding); err != nil {
				return nil, fmt.Errorf("failed to recompute holding %s: %w", symbol, err)
			}
		}
	}

	if !sawHeader {
		return nil, wrongFileError("holdings")
	}

	// Cash is the account's own, so it's recorded only when the file is
	// attributable to one: uploaded against an account, and covering a single
	// one. An all-accounts export would otherwise pool every account's cash
	// into whichever one it was uploaded to.
	if opts.AccountID != uuid.Nil && len(accountNumbers) == 1 {
		if err := portfolio.SetAccountCash(db, opts.AccountID, cash, asOf); err != nil {
			return nil, fmt.Errorf("failed to record cash balance: %w", err)
		}
	}
	// Holdings are shared, so the new prices move every account that holds
	// one of these symbols - not just the one the file was uploaded to.
	if err := portfolio.RecalcAllAccountBalances(db); err != nil {
		return nil, fmt.Errorf("failed to recompute account balances: %w", err)
	}

	// Log the file now that it's been recognized and read. A positions export
	// produces no UploadTransaction rows, so this Upload row is the only record
	// that the import happened - without it the file leaves no trace in history.
	//
	// It carries no date range: a positions file is a snapshot of one moment,
	// not a span of activity.
	if _, err := recordUpload(db, FileTypeHoldings, opts, nil, nil); err != nil {
		return nil, err
	}
	return result, nil
}

// downloadedDate reads the snapshot date from the export's footer line.
func downloadedDate(record []string) (time.Time, bool) {
	m := downloadedRegex.FindStringSubmatch(strings.Join(record, ","))
	if m == nil {
		return time.Time{}, false
	}
	d, err := time.Parse("Jan-02-2006", m[1])
	if err != nil {
		return time.Time{}, false
	}
	return d, true
}

// pendingActivity reports the value of a "Pending activity" row: cash already
// committed to a trade that hasn't settled. A buy shows up as a position at
// once while the money market still holds the cash, so this (negative) row is
// what keeps the purchase from being counted twice. The row stops short after
// its value, well under a position row's width.
func pendingActivity(record []string) (float64, bool) {
	if len(record) <= fidColCurrentValue ||
		!strings.EqualFold(strings.TrimSpace(record[fidColSymbol]), pendingActivitySymbol) {
		return 0, false
	}
	return parseDollar(record[fidColCurrentValue]), true
}

// parseDollar converts values like "$99.90" or "+$0.041" to a float,
// returning 0 for empty or unparseable values.
func parseDollar(s string) float64 {
	s = strings.NewReplacer("$", "", ",", "", "+", "").Replace(strings.TrimSpace(s))
	if s == "" || s == "--" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseDollarPtr is like parseDollar, but returns nil for empty or
// unparseable values instead of 0, so callers can store a real NULL rather
// than a made-up zero.
func parseDollarPtr(s string) *float64 {
	cleaned := strings.NewReplacer("$", "", ",", "", "+", "").Replace(strings.TrimSpace(s))
	if cleaned == "" || cleaned == "--" {
		return nil
	}
	v, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return nil
	}
	return &v
}
