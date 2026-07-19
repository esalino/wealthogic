package uploads

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// Column indices in a Fidelity "Portfolio Positions" export.
const (
	fidColSymbol      = 2
	fidColDescription = 3
	fidColLastPrice   = 5
	fidMinColumns     = 16
)

// assetTypeRules maps a substring of the Fidelity description to an asset
// type. Rules are checked in order; the first match wins.
var assetTypeRules = []struct {
	substr    string
	assetType string
}{
	{"MONEY MARKET", "Money Market"},
	{"TREAS", "Treasury"},
	{" PUT", "Option"},
	{" CALL", "Option"},
	{"ETF", "ETF"},
}

const defaultAssetType = "Stock"

func assetTypeFor(description string) string {
	desc := strings.ToUpper(description)
	for _, rule := range assetTypeRules {
		if strings.Contains(desc, rule.substr) {
			return rule.assetType
		}
	}
	return defaultAssetType
}

// fidelityHoldingsHandler parses a Fidelity portfolio positions CSV and
// upserts one Holding per symbol.
type fidelityHoldingsHandler struct{}

func (h *fidelityHoldingsHandler) Process(db *gorm.DB, file io.Reader) (*Result, error) {
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

		lastPrice := parseDollar(record[fidColLastPrice])

		var holding models.Holding
		err = db.Where("symbol = ?", symbol).First(&holding).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			holding = models.Holding{
				AssetType:   assetTypeFor(description),
				Symbol:      symbol,
				Description: description,
				LastPrice:   lastPrice,
			}
			if err := db.Create(&holding).Error; err != nil {
				return nil, fmt.Errorf("failed to create holding %s: %w", symbol, err)
			}
			result.Created++
		case err != nil:
			return nil, fmt.Errorf("failed to look up holding %s: %w", symbol, err)
		default:
			holding.AssetType = assetTypeFor(description)
			holding.Description = description
			holding.LastPrice = lastPrice
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
