package uploads

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// Column indices in a Fidelity "Portfolio Positions" export.
const (
	fidColSymbol       = 2
	fidColDescription  = 3
	fidColLastPrice    = 5
	fidColCurrentValue = 7
	fidMinColumns      = 16
)

// optionWordRegex matches PUT/CALL as whole words so it doesn't misfire on
// unrelated substrings, and matches whether the word leads the description
// (as in transaction rows, e.g. "PUT (MAR) MARRIOTT...") or trails it (as in
// holding rows, e.g. "MAR JUL 17 2026 $320 PUT").
var optionWordRegex = regexp.MustCompile(`\b(PUT|CALL)\b`)

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

func (h *fidelityHoldingsHandler) Process(db *gorm.DB, file io.Reader, _ Options) (*Result, error) {
	reader := csv.NewReader(file)
	// The export has a trailing disclaimer and footer rows with varying
	// column counts, so don't enforce a fixed number of fields.
	reader.FieldsPerRecord = -1

	result := &Result{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse csv: %w", err)
		}

		if len(record) < fidMinColumns {
			result.Skipped++
			continue
		}

		symbol := strings.TrimSuffix(strings.TrimSpace(record[fidColSymbol]), "**")
		description := strings.TrimSpace(record[fidColDescription])
		if symbol == "" || symbol == "Symbol" || description == "" {
			result.Skipped++
			continue
		}

		assetType := assetTypeFor(description)
		lastPrice := parseDollar(record[fidColLastPrice])

		// Cash-like holdings (e.g. money market) don't trade at a share
		// price, so their value comes straight from the CSV's Current Value
		// column instead of being derived from quantity * last price.
		var currentValue float64
		if assetType == cashAssetType {
			currentValue = parseDollar(record[fidColCurrentValue])
		}

		var holding models.Holding
		err = db.Where("symbol = ?", symbol).First(&holding).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			holding = models.Holding{
				AssetType:    assetType,
				Symbol:       symbol,
				Description:  description,
				LastPrice:    lastPrice,
				CurrentValue: currentValue,
			}
			if err := db.Create(&holding).Error; err != nil {
				return nil, fmt.Errorf("failed to create holding %s: %w", symbol, err)
			}
			result.Created++
		case err != nil:
			return nil, fmt.Errorf("failed to look up holding %s: %w", symbol, err)
		default:
			holding.AssetType = assetType
			holding.Description = description
			holding.LastPrice = lastPrice
			holding.CurrentValue = currentValue
			if err := db.Save(&holding).Error; err != nil {
				return nil, fmt.Errorf("failed to update holding %s: %w", symbol, err)
			}
			result.Updated++
		}
	}

	return result, nil
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
