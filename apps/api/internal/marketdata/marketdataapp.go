package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// marketDataAppClient reads quotes from marketdata.app.
//
// Quotes only: the vendor prices symbols FMP's free tier refuses (it answers
// 402 Payment Required off the major exchanges) but says nothing about the
// company behind them, so it implements QuoteProvider and not ProfileProvider.
type marketDataAppClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewMarketDataAppClient builds a quote provider against marketdata.app. It
// returns nil when no API key is configured.
func NewMarketDataAppClient(apiKey string) QuoteProvider {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &marketDataAppClient{
		apiKey:  apiKey,
		baseURL: "https://api.marketdata.app/v1",
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *marketDataAppClient) Name() string { return ProviderMarketDataApp }

// Status values the vendor returns in the "s" field, which is authoritative -
// it distinguishes a symbol it has no price for from a request that was wrong.
const (
	mdaStatusOK     = "ok"
	mdaStatusNoData = "no_data"
)

// mdaQuote is the vendor's shape. Every field is an array, one entry per symbol
// requested, so a single-symbol lookup reads index 0.
type mdaQuote struct {
	Status string   `json:"s"`
	ErrMsg string   `json:"errmsg"`
	Symbol []string `json:"symbol"`
	// Last is the last traded price, which is the one that belongs on a holding.
	// Bid and ask bracket it and mid is their average; none of those is a trade
	// that happened.
	Last []float64 `json:"last"`
	// Updated is unix seconds of the quote.
	Updated []int64 `json:"updated"`
}

func (c *marketDataAppClient) FetchQuote(ctx context.Context, symbol string) (Quote, bool, error) {
	// The symbol is a path segment here rather than a query parameter, so it is
	// path-escaped - a symbol like BRK.A must survive intact.
	endpoint := fmt.Sprintf("%s/stocks/quotes/%s/?format=json&token=%s",
		c.baseURL, url.PathEscape(symbol), url.QueryEscape(c.apiKey))

	status, body, err := fetchJSON(ctx, c.http, endpoint)
	if err != nil {
		return Quote{}, false, err
	}

	// 404 is this vendor's "I don't have that symbol", which is an answer rather
	// than a failure - and its body is not worth decoding to learn that.
	if status == http.StatusNotFound {
		return Quote{}, false, nil
	}

	// 200 is a live quote and 203 a cached one. Both carry real prices, and
	// rejecting 203 would drop every delayed symbol - BRK.A answers 203.
	if status != http.StatusOK && status != http.StatusNonAuthoritativeInfo {
		// The body names the problem ("Bad parameters", a plan limit) far more
		// usefully than the status alone, so it travels with the error. It is
		// the vendor's own message and carries no credentials.
		var out mdaQuote
		if err := json.Unmarshal(body, &out); err == nil && out.ErrMsg != "" {
			return Quote{}, false, fmt.Errorf("%s quote lookup for %s: HTTP %d: %s",
				ProviderMarketDataApp, symbol, status, out.ErrMsg)
		}
		return Quote{}, false, statusError(ProviderMarketDataApp, "quote", symbol, status)
	}

	var out mdaQuote
	if err := json.Unmarshal(body, &out); err != nil {
		return Quote{}, false, fmt.Errorf("%s quote lookup for %s: %w", ProviderMarketDataApp, symbol, err)
	}

	// The status field decides, not the HTTP code: a 200 carrying "no_data" is
	// still no data.
	if out.Status == mdaStatusNoData {
		return Quote{}, false, nil
	}
	if out.Status != mdaStatusOK {
		if out.ErrMsg != "" {
			return Quote{}, false, fmt.Errorf("%s quote lookup for %s: %s",
				ProviderMarketDataApp, symbol, out.ErrMsg)
		}
		return Quote{}, false, fmt.Errorf("%s quote lookup for %s: status %q",
			ProviderMarketDataApp, symbol, out.Status)
	}

	// A well-formed "ok" with no price in it is not something to write over a
	// good price with.
	if len(out.Last) == 0 || out.Last[0] == 0 {
		return Quote{}, false, nil
	}

	quote := Quote{Symbol: symbol, Price: out.Last[0]}
	// The vendor echoes the symbol it matched; prefer it when present.
	if len(out.Symbol) > 0 && out.Symbol[0] != "" {
		quote.Symbol = out.Symbol[0]
	}
	if len(out.Updated) > 0 && out.Updated[0] > 0 {
		quote.AsOf = time.Unix(out.Updated[0], 0).UTC()
	}
	return quote, true, nil
}
