// Package marketdata reads from an external market-data provider: reference
// data about a security - what company it is, what sector and industry it
// trades in - and its latest quoted price.
//
// It is deliberately best-effort. Nothing in the portfolio or tax math depends
// on a lookup succeeding: reference data is descriptive, and a price is a
// snapshot that was already going stale when it arrived. A provider being slow,
// rate-limited, or unreachable must never stop a holding being created.
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

// Profile is what a provider can tell us about a symbol. Every field is
// optional; a provider that knows nothing returns ok=false rather than a
// zero-valued profile, so "not found" and "found but blank" stay distinct.
type Profile struct {
	Symbol      string `json:"symbol"`
	CompanyName string `json:"company_name"`
	Sector      string `json:"sector"`
	Industry    string `json:"industry"`
	Exchange    string `json:"exchange"`
	Country     string `json:"country"`
	Website     string `json:"website"`
	LogoURL     string `json:"logo_url"`
	// Description is the provider's prose summary of the business - several
	// paragraphs, not a label.
	Description string `json:"description"`
}

// Quote is a symbol's latest traded price.
type Quote struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	// AsOf is when the exchange last traded at this price, which is not when we
	// asked: a quote pulled on a Sunday is Friday's close. Zero when the
	// provider didn't say, and callers then fall back to the time of the call.
	AsOf time.Time `json:"as_of"`
}

// Provider looks up a symbol's market data. One implementation today; the
// interface is here so the enrichment and repricing can be tested without a
// network.
type Provider interface {
	// FetchProfile returns the symbol's profile. ok is false when the provider
	// has no record of the symbol, which is not an error - a treasury CUSIP or
	// an option contract simply isn't a company.
	FetchProfile(ctx context.Context, symbol string) (profile Profile, ok bool, err error)

	// FetchQuote returns the symbol's latest price. ok is false when the
	// provider doesn't cover the symbol or has no price for it, which is not an
	// error either - a delisted ticker still has holdings behind it.
	FetchQuote(ctx context.Context, symbol string) (quote Quote, ok bool, err error)
}

// fmpClient reads from Financial Modeling Prep.
type fmpClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewFMPClient builds a provider against Financial Modeling Prep. It returns
// nil when no API key is configured, which callers treat as "market data is
// switched off" - the app runs perfectly well without it.
func NewFMPClient(apiKey string) Provider {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &fmpClient{
		apiKey:  apiKey,
		baseURL: "https://financialmodelingprep.com/stable",
		// Market data is a nice-to-have on a request path that must stay
		// responsive, so the wait is short and failure is tolerated.
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// get calls one of the provider's endpoints for a symbol and decodes the array
// it answers with into out, which must be a pointer to a slice. It reports
// whether the array held anything: the endpoints return an empty one for a
// symbol they don't cover, which is an answer rather than a failure.
func (c *fmpClient) get(ctx context.Context, path, symbol string, out any) (bool, error) {
	endpoint := fmt.Sprintf("%s/%s?symbol=%s&apikey=%s",
		c.baseURL, path, url.QueryEscape(symbol), url.QueryEscape(c.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The key and the URL are not worth repeating into logs, so the error
		// carries the status and the symbol only.
		return false, fmt.Errorf("%s lookup for %s: %s", path, symbol, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, fmt.Errorf("%s lookup for %s: %w", path, symbol, err)
	}
	return true, nil
}

// fmpProfile is the provider's shape, mapped to ours so the rest of the app
// doesn't take on a vendor's field names.
type fmpProfile struct {
	Symbol      string `json:"symbol"`
	CompanyName string `json:"companyName"`
	Sector      string `json:"sector"`
	Industry    string `json:"industry"`
	Exchange    string `json:"exchange"`
	Country     string `json:"country"`
	Website     string `json:"website"`
	Image       string `json:"image"`
	Description string `json:"description"`
}

func (c *fmpClient) FetchProfile(ctx context.Context, symbol string) (Profile, bool, error) {
	var out []fmpProfile
	if _, err := c.get(ctx, "profile", symbol, &out); err != nil {
		return Profile{}, false, err
	}
	if len(out) == 0 {
		return Profile{}, false, nil
	}

	p := out[0]
	return Profile{
		Symbol:      p.Symbol,
		CompanyName: p.CompanyName,
		Sector:      p.Sector,
		Industry:    p.Industry,
		Exchange:    p.Exchange,
		Country:     p.Country,
		Website:     p.Website,
		LogoURL:     p.Image,
		Description: p.Description,
	}, true, nil
}

// fmpQuote is the quote endpoint's shape. It carries a few dozen fields; only
// the price and the moment it was traded at matter here.
type fmpQuote struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	// Timestamp is unix seconds of the last trade.
	Timestamp int64 `json:"timestamp"`
}

func (c *fmpClient) FetchQuote(ctx context.Context, symbol string) (Quote, bool, error) {
	var out []fmpQuote
	if _, err := c.get(ctx, "quote", symbol, &out); err != nil {
		return Quote{}, false, err
	}
	if len(out) == 0 {
		return Quote{}, false, nil
	}

	q := out[0]
	// A row with no price is no more use than no row at all, and treating it as
	// found would overwrite a good price with zero.
	if q.Price == 0 {
		return Quote{}, false, nil
	}

	quote := Quote{Symbol: q.Symbol, Price: q.Price}
	if q.Timestamp > 0 {
		quote.AsOf = time.Unix(q.Timestamp, 0).UTC()
	}
	return quote, true, nil
}
