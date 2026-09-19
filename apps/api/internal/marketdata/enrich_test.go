package marketdata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
)

// fakeProvider answers from a map and counts what it was asked, so the tests
// can assert on calls that should never have been made.
type fakeProvider struct {
	profiles map[string]Profile
	err      error
	asked    []string
}

func (f *fakeProvider) FetchProfile(_ context.Context, symbol string) (Profile, bool, error) {
	f.asked = append(f.asked, symbol)
	if f.err != nil {
		return Profile{}, false, f.err
	}
	p, ok := f.profiles[symbol]
	return p, ok, nil
}

// Every provider call is rate-limited, so the ones that cannot possibly return
// anything are worth not making: an option is a contract rather than a company,
// and a treasury is government debt identified by CUSIP.
func TestWantsProfile(t *testing.T) {
	fetched := time.Now()
	tests := []struct {
		name    string
		holding models.Holding
		want    bool
	}{
		{"a stock is worth looking up", models.Holding{AssetType: models.AssetTypeStock, Symbol: "AAPL"}, true},
		{"so is an ETF", models.Holding{AssetType: models.AssetTypeETF, Symbol: "TLT"}, true},
		{"and a money-market fund", models.Holding{AssetType: models.AssetTypeMoneyMarket, Symbol: "FZFXX"}, true},
		{"a treasury is not a company", models.Holding{AssetType: models.AssetTypeTreasury, Symbol: "912797RS8"}, false},
		{"an option is a contract", models.Holding{AssetType: models.AssetTypeOption, Symbol: "-AXP251121C390"}, false},
		{
			// Even mis-typed, the symbol itself gives it away.
			name:    "an option symbol is caught whatever the asset type says",
			holding: models.Holding{AssetType: models.AssetTypeStock, Symbol: "-AXP251121C390"},
			want:    false,
		},
		{"nothing to look up without a symbol", models.Holding{AssetType: models.AssetTypeStock}, false},
		{
			name:    "already asked, even if it found nothing",
			holding: models.Holding{AssetType: models.AssetTypeStock, Symbol: "AAPL", ProfileFetchedAt: &fetched},
			want:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.holding.WantsProfile(); got != tc.want {
				t.Errorf("WantsProfile() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil enricher is what a build with no API key produces, and every path
// through it has to stay a no-op rather than panicking.
func TestNilEnricherIsSafe(t *testing.T) {
	var e *Enricher
	if e.Enabled() {
		t.Error("a nil enricher should not report itself enabled")
	}
	if err := e.Enrich(context.Background(), nil, &models.Holding{Symbol: "AAPL"}); err != nil {
		t.Errorf("Enrich on a nil enricher returned %v, want nil", err)
	}
	e.EnrichQuietly(context.Background(), nil, &models.Holding{Symbol: "AAPL"})

	if got := NewEnricher(nil); got != nil {
		t.Error("NewEnricher(nil) should yield a nil enricher")
	}
	if got := NewFMPClient("   "); got != nil {
		t.Error("a blank API key should yield no client")
	}
}

// A provider failure must surface as an error the caller can log, never as a
// reason the holding itself fails.
func TestEnrichSurfacesProviderError(t *testing.T) {
	p := &fakeProvider{err: errors.New("rate limited")}
	e := NewEnricher(p)

	err := e.Enrich(context.Background(), nil, &models.Holding{AssetType: models.AssetTypeStock, Symbol: "AAPL"})
	if err == nil {
		t.Fatal("expected the provider error to surface")
	}
	if len(p.asked) != 1 {
		t.Errorf("provider asked %d times, want 1", len(p.asked))
	}
}

// Skipped holdings must not reach the provider at all.
func TestEnrichSkipsIneligible(t *testing.T) {
	p := &fakeProvider{profiles: map[string]Profile{}}
	e := NewEnricher(p)

	for _, h := range []models.Holding{
		{AssetType: models.AssetTypeTreasury, Symbol: "912797RS8"},
		{AssetType: models.AssetTypeOption, Symbol: "-AXP251121C390"},
		{AssetType: models.AssetTypeStock},
	} {
		if err := e.Enrich(context.Background(), nil, &h); err != nil {
			t.Errorf("Enrich(%s) = %v, want nil", h.Symbol, err)
		}
	}
	if len(p.asked) != 0 {
		t.Errorf("provider was asked about %v, want nothing", p.asked)
	}
}
