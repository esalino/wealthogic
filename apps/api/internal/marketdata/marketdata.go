// Package marketdata fetches reference data about a security - what company it
// is, what sector and industry it trades in - from an external provider.
//
// It is deliberately best-effort. The data is descriptive rather than financial:
// nothing in the portfolio or tax math depends on it, so a provider being slow,
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

// Provider looks up a symbol's reference data. One implementation today; the
// interface is here so the enrichment can be tested without a network.
type Provider interface {
	// FetchProfile returns the symbol's profile. ok is false when the provider
	// has no record of the symbol, which is not an error - a treasury CUSIP or
	// an option contract simply isn't a company.
	FetchProfile(ctx context.Context, symbol string) (profile Profile, ok bool, err error)
}

// fmpClient reads from Financial Modeling Prep.
type fmpClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewFMPClient builds a provider against Financial Modeling Prep. It returns
// nil when no API key is configured, which callers treat as "enrichment is
// switched off" - the app runs perfectly well without it.
func NewFMPClient(apiKey string) Provider {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &fmpClient{
		apiKey:  apiKey,
		baseURL: "https://financialmodelingprep.com/stable/profile",
		// Reference data is a nice-to-have on a request path that must stay
		// responsive, so the wait is short and failure is tolerated.
		http: &http.Client{Timeout: 10 * time.Second},
	}
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
	endpoint := fmt.Sprintf("%s?symbol=%s&apikey=%s",
		c.baseURL, url.QueryEscape(symbol), url.QueryEscape(c.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Profile{}, false, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Profile{}, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The key and the URL are not worth repeating into logs, so the error
		// carries the status and the symbol only.
		return Profile{}, false, fmt.Errorf("profile lookup for %s: %s", symbol, resp.Status)
	}

	// The endpoint answers with an array, empty for a symbol it doesn't cover.
	var out []fmpProfile
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Profile{}, false, fmt.Errorf("profile lookup for %s: %w", symbol, err)
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
