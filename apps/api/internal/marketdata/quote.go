package marketdata

import (
	"context"
	"log"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"gorm.io/gorm"
)

// Pricer refreshes holdings' quoted prices from a provider.
//
// Like the Enricher, every method is safe to call on a nil Pricer, which is
// what a build with no API key configured produces.
type Pricer struct {
	provider Provider
}

// NewPricer wraps a provider. A nil provider yields a nil Pricer, so "no key
// configured" and "no pricer" are the same thing to callers.
func NewPricer(provider Provider) *Pricer {
	if provider == nil {
		return nil
	}
	return &Pricer{provider: provider}
}

// Enabled reports whether repricing will actually do anything.
func (p *Pricer) Enabled() bool { return p != nil && p.provider != nil }

// quoteOutcome is what a refresh managed for one holding.
type quoteOutcome string

const (
	quotePriced   quoteOutcome = "priced"
	quoteNotFound quoteOutcome = "not_found"
	quoteFailed   quoteOutcome = "failed"
)

// QuotedSymbol is one line of the refresh's log, so the caller can see which
// symbols moved and which the provider had nothing for.
type QuotedSymbol struct {
	Symbol string `json:"symbol"`
	// Outcome is "priced", "not_found", or "failed".
	Outcome string `json:"outcome"`
	// Price and AsOf are set only for a symbol that was priced.
	Price float64    `json:"price"`
	AsOf  *time.Time `json:"as_of"`
	// Previous is the price this replaced, so an obviously wrong quote is
	// visible as a jump rather than having to be inferred.
	Previous float64 `json:"previous_price"`
}

// RefreshResult reports what a refresh pass did.
type RefreshResult struct {
	Considered int `json:"considered"`
	Priced     int `json:"priced"`
	NotFound   int `json:"not_found"`
	Failed     int `json:"failed"`
	// FetchedAt is when the pass ran, which is not what any one price is as of -
	// each holding carries its own last_price_updated_at.
	FetchedAt time.Time      `json:"fetched_at"`
	Symbols   []QuotedSymbol `json:"symbols"`
} // @name PriceRefreshResult

// RefreshHolding quotes one holding and saves the new price along with the
// figures that follow from it. It reports whether a price was actually written.
func (p *Pricer) RefreshHolding(ctx context.Context, db *gorm.DB, holding *models.Holding) (bool, error) {
	if !p.Enabled() || !holding.WantsQuote() {
		return false, nil
	}

	quote, found, err := p.provider.FetchQuote(ctx, holding.Symbol)
	if err != nil {
		return false, err
	}
	if !found {
		// Unlike a profile, a missing quote isn't a settled answer: a symbol the
		// provider can't price today may well price tomorrow. Nothing is stamped,
		// so the next pass asks again.
		return false, nil
	}

	// A quote with no timestamp of its own is as of now: it's the latest the
	// provider has, and claiming no time at all would leave the holding looking
	// as though it had never been priced.
	asOf := quote.AsOf
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}

	portfolio.ApplyPrice(holding, quote.Price, asOf)

	return true, db.Model(holding).Updates(map[string]any{
		"last_price":              holding.LastPrice,
		"last_price_updated_at":   holding.LastPriceUpdatedAt,
		"current_value":           holding.CurrentValue,
		"gain_unrealized_amount":  holding.GainUnrealizedAmount,
		"gain_unrealized_percent": holding.GainUnrealizedPercent,
	}).Error
}

// RefreshOpenPositions quotes every holding that wants a price and saves what
// comes back.
//
// Requests go one at a time with a pause between them: providers rate-limit,
// and whoever asked for the refresh can wait. A failure stops nothing - it's
// counted and the pass moves on, so one bad symbol can't strand the rest.
func (p *Pricer) RefreshOpenPositions(ctx context.Context, db *gorm.DB, pause time.Duration) (RefreshResult, error) {
	result := RefreshResult{FetchedAt: time.Now().UTC(), Symbols: []QuotedSymbol{}}
	if !p.Enabled() {
		return result, nil
	}

	// Narrowed in SQL as well as by WantsQuote, so a large book of closed
	// positions and options isn't loaded just to be skipped.
	var holdings []models.Holding
	if err := db.Where("status = ? AND asset_type = ? AND symbol <> ''",
		models.HoldingStatusOpen, models.AssetTypeStock).
		Order("symbol ASC").Find(&holdings).Error; err != nil {
		return result, err
	}

	for i := range holdings {
		h := &holdings[i]
		if !h.WantsQuote() {
			continue
		}
		result.Considered++

		if err := ctx.Err(); err != nil {
			return result, err
		}
		if result.Considered > 1 && pause > 0 {
			time.Sleep(pause)
		}

		line := QuotedSymbol{Symbol: h.Symbol, Previous: h.LastPrice}
		priced, err := p.RefreshHolding(ctx, db, h)
		switch {
		case err != nil:
			log.Printf("market data: quote for %s failed: %v", h.Symbol, err)
			line.Outcome = string(quoteFailed)
			result.Failed++
		case priced:
			line.Outcome = string(quotePriced)
			line.Price = h.LastPrice
			line.AsOf = h.LastPriceUpdatedAt
			result.Priced++
		default:
			line.Outcome = string(quoteNotFound)
			result.NotFound++
		}
		result.Symbols = append(result.Symbols, line)
	}
	return result, nil
}
