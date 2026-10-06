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

// fmpClient reads from Financial Modeling Prep.
//
// It serves both halves of the package: its profile data is what the holdings'
// sector and industry come from, and it can quote prices too - though its free
// tier answers 402 for symbols off the major exchanges, which is why quotes are
// swappable.
type fmpClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewFMPClient builds a provider against Financial Modeling Prep. It returns
// nil when no API key is configured, which callers treat as "this vendor is
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

func (c *fmpClient) Name() string { return ProviderFMP }

// get calls one of FMP's endpoints for a symbol and decodes the array it
// answers with into out, which must be a pointer to a slice.
func (c *fmpClient) get(ctx context.Context, path, symbol string, out any) error {
	endpoint := fmt.Sprintf("%s/%s?symbol=%s&apikey=%s",
		c.baseURL, path, url.QueryEscape(symbol), url.QueryEscape(c.apiKey))

	status, body, err := fetchJSON(ctx, c.http, endpoint)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError(ProviderFMP, path, symbol, status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s %s lookup for %s: %w", ProviderFMP, path, symbol, err)
	}
	return nil
}

// fmpProfile is the vendor's shape, mapped to ours so the rest of the app
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
	// The endpoint answers with an array, empty for a symbol it doesn't cover.
	var out []fmpProfile
	if err := c.get(ctx, "profile", symbol, &out); err != nil {
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
	if err := c.get(ctx, "quote", symbol, &out); err != nil {
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
