package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mdaServer stands in for the vendor, answering every request with one canned
// status and body and recording the path it was asked for.
func mdaServer(t *testing.T, status int, body string) (*marketDataAppClient, *string) {
	t.Helper()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return &marketDataAppClient{apiKey: "test-key", baseURL: srv.URL + "/v1", http: srv.Client()}, &gotPath
}

// The happy path: the vendor answers in column-oriented arrays, and `last` is
// the traded price that belongs on a holding.
func TestMarketDataAppFetchQuote(t *testing.T) {
	c, path := mdaServer(t, http.StatusOK,
		`{"s":"ok","symbol":["EVGO"],"bid":[1.31],"ask":[1.35],"mid":[1.33],"last":[1.32],"updated":[1791316863]}`)

	quote, ok, err := c.FetchQuote(context.Background(), "EVGO")
	if err != nil || !ok {
		t.Fatalf("FetchQuote = (%v, %v, %v), want a quote", quote, ok, err)
	}
	if quote.Price != 1.32 {
		t.Errorf("price = %v, want 1.32 (last, not bid/ask/mid)", quote.Price)
	}
	if quote.Symbol != "EVGO" {
		t.Errorf("symbol = %q, want EVGO", quote.Symbol)
	}
	if got := quote.AsOf.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-10-06T20:01:03Z" {
		t.Errorf("as-of = %s, want the unix timestamp decoded", got)
	}
	// The token belongs in the query, the symbol in the path.
	if !strings.Contains(*path, "/v1/stocks/quotes/EVGO/") {
		t.Errorf("requested %q, want the symbol as a path segment", *path)
	}
}

// 203 is this vendor's "cached, not live". The data is real and rejecting it
// would silently drop every delayed symbol - BRK.A answers 203 in practice.
func TestMarketDataAppAcceptsCachedQuote(t *testing.T) {
	c, _ := mdaServer(t, http.StatusNonAuthoritativeInfo,
		`{"s":"ok","symbol":["BRK.A"],"last":[758166],"updated":[1791316863]}`)

	quote, ok, err := c.FetchQuote(context.Background(), "BRK.A")
	if err != nil || !ok {
		t.Fatalf("FetchQuote on a 203 = (%v, %v), want the quote accepted", ok, err)
	}
	if quote.Price != 758166 {
		t.Errorf("price = %v, want 758166", quote.Price)
	}
}

// A symbol with a dot has to survive being put in the path.
func TestMarketDataAppEscapesSymbol(t *testing.T) {
	c, path := mdaServer(t, http.StatusOK, `{"s":"ok","symbol":["BRK.A"],"last":[758166]}`)

	if _, _, err := c.FetchQuote(context.Background(), "BRK.A"); err != nil {
		t.Fatalf("FetchQuote = %v, want nil", err)
	}
	if !strings.Contains(*path, "BRK.A") {
		t.Errorf("requested %q, want the symbol intact", *path)
	}
}

// A symbol the vendor has no price for is an answer, not a failure: nothing is
// stamped so the next pass asks again.
func TestMarketDataAppNotFound(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"404 with no_data", http.StatusNotFound, `{"s":"no_data","errmsg":"Symbol not found."}`},
		{"200 carrying no_data", http.StatusOK, `{"s":"no_data"}`},
		{"ok with no price in it", http.StatusOK, `{"s":"ok","symbol":["ZZZ"],"last":[]}`},
		{"ok with a zero price", http.StatusOK, `{"s":"ok","symbol":["ZZZ"],"last":[0]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := mdaServer(t, tc.status, tc.body)

			quote, ok, err := c.FetchQuote(context.Background(), "ZZZ")
			if err != nil {
				t.Fatalf("FetchQuote = %v, want nil - not found is not an error", err)
			}
			if ok {
				t.Errorf("FetchQuote reported a quote (%v), want none", quote)
			}
		})
	}
}

// A real failure has to surface, carrying the vendor's own message - which says
// far more than the status alone about a plan limit or a bad symbol.
func TestMarketDataAppSurfacesError(t *testing.T) {
	c, _ := mdaServer(t, http.StatusBadRequest,
		`{"s":"error","errmsg":"Bad parameters, please check API documentation."}`)

	_, ok, err := c.FetchQuote(context.Background(), "ZZZZQQQQ")
	if err == nil {
		t.Fatal("expected the vendor error to surface")
	}
	if ok {
		t.Error("a failed lookup must not report a quote")
	}
	if !strings.Contains(err.Error(), "Bad parameters") {
		t.Errorf("error = %q, want the vendor's message included", err)
	}
	if !strings.Contains(err.Error(), ProviderMarketDataApp) {
		t.Errorf("error = %q, want the provider named", err)
	}
}

// A blank key means the vendor is switched off, and must read as a nil
// interface rather than a non-nil one wrapping nothing.
func TestMarketDataAppBlankKeyYieldsNoClient(t *testing.T) {
	if got := NewMarketDataAppClient("   "); got != nil {
		t.Error("a blank API key should yield no client")
	}
}

// Swapping vendors is meant to be a configuration change, so the selection has
// to be predictable - including the fallbacks.
func TestNewQuoteProvider(t *testing.T) {
	tests := []struct {
		name string
		cfg  QuoteConfig
		want string // "" means no provider
	}{
		{
			name: "named provider wins",
			cfg:  QuoteConfig{Provider: ProviderFMP, FMPAPIKey: "f", MarketDataAPIKey: "m"},
			want: ProviderFMP,
		},
		{
			name: "named provider wins the other way too",
			cfg:  QuoteConfig{Provider: ProviderMarketDataApp, FMPAPIKey: "f", MarketDataAPIKey: "m"},
			want: ProviderMarketDataApp,
		},
		{
			name: "the name is case- and space-insensitive",
			cfg:  QuoteConfig{Provider: "  MarketData.App  ", MarketDataAPIKey: "m"},
			want: ProviderMarketDataApp,
		},
		{
			// Broader free-tier coverage, so it's the better default.
			name: "unconfigured prefers marketdata.app",
			cfg:  QuoteConfig{FMPAPIKey: "f", MarketDataAPIKey: "m"},
			want: ProviderMarketDataApp,
		},
		{
			// An existing deployment with only an FMP key keeps working.
			name: "unconfigured falls back to the key it has",
			cfg:  QuoteConfig{FMPAPIKey: "f"},
			want: ProviderFMP,
		},
		{
			name: "a named provider with no key is switched off",
			cfg:  QuoteConfig{Provider: ProviderMarketDataApp, FMPAPIKey: "f"},
			want: "",
		},
		{"no keys at all", QuoteConfig{}, ""},
		{"an unknown name selects nothing", QuoteConfig{Provider: "nope", FMPAPIKey: "f"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewQuoteProvider(tc.cfg)
			if tc.want == "" {
				// Must be a genuinely nil interface: the Pricer's nil check
				// depends on it, and a typed nil would pass and then panic.
				if got != nil {
					t.Fatalf("NewQuoteProvider = %q, want nil", got.Name())
				}
				if NewPricer(got).Enabled() {
					t.Error("a nil provider should yield a disabled pricer")
				}
				return
			}
			if got == nil {
				t.Fatalf("NewQuoteProvider = nil, want %s", tc.want)
			}
			if got.Name() != tc.want {
				t.Errorf("provider = %q, want %q", got.Name(), tc.want)
			}
		})
	}
}
