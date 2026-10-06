// Package marketdata reads from external market-data providers: reference data
// about a security - what company it is, what sector and industry it trades in -
// and its latest quoted price.
//
// Profiles and quotes are deliberately separate concerns, because no single free
// tier covers both well: one vendor may describe companies richly but refuse to
// price anything off the major exchanges, while another prices everything and
// knows nothing about the business. So the two are separate interfaces, a vendor
// implements whichever it is good at, and which one quotes prices is chosen at
// startup (see NewQuoteProvider).
//
// It is all deliberately best-effort. Nothing in the portfolio or tax math
// depends on a lookup succeeding: reference data is descriptive, and a price is
// a snapshot that was already going stale when it arrived. A provider being
// slow, rate-limited, or unreachable must never stop a holding being created.
package marketdata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider names, as they appear in logs, in the refresh result, and in the
// QUOTE_PROVIDER environment variable.
const (
	ProviderFMP           = "fmp"
	ProviderMarketDataApp = "marketdata.app"
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

// ProfileProvider looks up a symbol's reference data.
type ProfileProvider interface {
	// Name identifies the vendor, for logs and for reporting which provider
	// produced a given figure.
	Name() string

	// FetchProfile returns the symbol's profile. ok is false when the provider
	// has no record of the symbol, which is not an error - a treasury CUSIP or
	// an option contract simply isn't a company.
	FetchProfile(ctx context.Context, symbol string) (profile Profile, ok bool, err error)
}

// Provider does both halves, which a vendor with good coverage of each can.
// FMP is one: it supplies the profiles and can quote prices too.
type Provider interface {
	ProfileProvider
	QuoteProvider
}

// QuoteProvider looks up a symbol's latest price.
//
// This is the interchangeable half of the package: swapping vendors means
// implementing this one method and adding a case to NewQuoteProvider, with
// nothing in the Pricer or the handlers to change.
type QuoteProvider interface {
	// Name identifies the vendor, for logs and for reporting which provider
	// priced the book.
	Name() string

	// FetchQuote returns the symbol's latest price. ok is false when the
	// provider doesn't cover the symbol or has no price for it, which is not an
	// error - a symbol off the major exchanges is a gap in a free tier rather
	// than a fault.
	FetchQuote(ctx context.Context, symbol string) (quote Quote, ok bool, err error)
}

// QuoteConfig is the environment's view of which provider should price the book
// and what credentials are available.
type QuoteConfig struct {
	// Provider names the one to use: ProviderFMP or ProviderMarketDataApp.
	// Empty picks whichever has a key configured, preferring marketdata.app -
	// its free tier covers symbols FMP's answers 402 for.
	Provider string

	FMPAPIKey        string
	MarketDataAPIKey string
}

// NewQuoteProvider builds the quote provider the configuration asks for.
//
// It returns nil when the named provider has no key - callers treat that as
// "repricing is switched off", the same as having no provider at all.
func NewQuoteProvider(cfg QuoteConfig) QuoteProvider {
	// Each constructor returns a literal nil when it has no key, and converting
	// that nil interface to a QuoteProvider leaves it nil - so an unconfigured
	// vendor reaches the caller as "switched off" rather than as a non-nil
	// interface wrapping nothing.
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case ProviderFMP:
		return NewFMPClient(cfg.FMPAPIKey)
	case ProviderMarketDataApp:
		return NewMarketDataAppClient(cfg.MarketDataAPIKey)
	case "":
		// Unconfigured: prefer the broader free tier, but fall back to whatever
		// has a key so an existing deployment keeps working untouched.
		if p := NewMarketDataAppClient(cfg.MarketDataAPIKey); p != nil {
			return p
		}
		return NewFMPClient(cfg.FMPAPIKey)
	}
	return nil
}

// fetchJSON performs a GET and returns the status and body.
//
// The status comes back rather than being checked here because the vendors
// disagree about what a status means: marketdata.app answers 203 for a cached
// quote (perfectly good data) and 404 for a symbol it can't price, while FMP
// answers 200 with an empty array. Each client reads its own vendor's dialect.
func fetchJSON(ctx context.Context, httpClient *http.Client, endpoint string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	// Capped: a provider answering with something enormous is a bug on their
	// side, and we would rather fail the one lookup than the process.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// statusError describes a failed lookup without repeating the URL or the key
// into the logs.
func statusError(provider, kind, symbol string, status int) error {
	return fmt.Errorf("%s %s lookup for %s: HTTP %d", provider, kind, symbol, status)
}
