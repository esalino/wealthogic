package marketdata

import (
	"context"
	"errors"
	"testing"

	"github.com/eriksalino/wealthogic/api/internal/models"
)

// Every quote is a rate-limited call, and a price only means something for a
// position that is still open.
func TestWantsQuote(t *testing.T) {
	tests := []struct {
		name    string
		holding models.Holding
		want    bool
	}{
		{"an open stock wants a price", models.Holding{AssetType: models.AssetTypeStock, Symbol: "AAPL", Status: models.HoldingStatusOpen}, true},
		{
			name:    "a closed position's figures are history",
			holding: models.Holding{AssetType: models.AssetTypeStock, Symbol: "AAPL", Status: models.HoldingStatusClosed},
			want:    false,
		},
		{
			// Quoted per contract symbol rather than per ticker.
			name:    "an option keeps the price its trade came in at",
			holding: models.Holding{AssetType: models.AssetTypeOption, Symbol: "-AXP251121C390", Status: models.HoldingStatusOpen},
			want:    false,
		},
		{
			name:    "an option symbol is caught whatever the asset type says",
			holding: models.Holding{AssetType: models.AssetTypeStock, Symbol: "-AXP251121C390", Status: models.HoldingStatusOpen},
			want:    false,
		},
		{
			name:    "a treasury is not quoted by ticker",
			holding: models.Holding{AssetType: models.AssetTypeTreasury, Symbol: "912797RS8", Status: models.HoldingStatusOpen},
			want:    false,
		},
		{"nothing to quote without a symbol", models.Holding{AssetType: models.AssetTypeStock, Status: models.HoldingStatusOpen}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.holding.WantsQuote(); got != tc.want {
				t.Errorf("WantsQuote() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil pricer is what a build with no API key produces, and every path through
// it has to stay a no-op rather than panicking.
func TestNilPricerIsSafe(t *testing.T) {
	var p *Pricer
	if p.Enabled() {
		t.Error("a nil pricer should not report itself enabled")
	}
	holding := models.Holding{AssetType: models.AssetTypeStock, Symbol: "AAPL", Status: models.HoldingStatusOpen}
	if priced, err := p.RefreshHolding(context.Background(), nil, &holding); priced || err != nil {
		t.Errorf("RefreshHolding on a nil pricer = (%v, %v), want (false, nil)", priced, err)
	}
	if got := NewPricer(nil); got != nil {
		t.Error("NewPricer(nil) should yield a nil pricer")
	}
}

// A provider failure has to surface as an error the caller can count, never as
// a price written anyway.
func TestRefreshHoldingSurfacesProviderError(t *testing.T) {
	p := NewPricer(&fakeProvider{err: errors.New("rate limited")})
	holding := models.Holding{AssetType: models.AssetTypeStock, Symbol: "AAPL", Status: models.HoldingStatusOpen, LastPrice: 100}

	priced, err := p.RefreshHolding(context.Background(), nil, &holding)
	if err == nil {
		t.Fatal("expected the provider error to surface")
	}
	if priced {
		t.Error("a failed lookup must not report a price")
	}
	if holding.LastPrice != 100 {
		t.Errorf("last price = %v, want the old 100 left alone", holding.LastPrice)
	}
}

// A symbol the provider can't price leaves the holding exactly as it was: the
// old price is still the best one we have, and nothing is stamped, so the next
// pass asks again.
func TestRefreshHoldingLeavesUnquotedSymbolAlone(t *testing.T) {
	p := NewPricer(&fakeProvider{quotes: map[string]Quote{}})
	holding := models.Holding{AssetType: models.AssetTypeStock, Symbol: "DELISTED", Status: models.HoldingStatusOpen, LastPrice: 42}

	priced, err := p.RefreshHolding(context.Background(), nil, &holding)
	if err != nil {
		t.Fatalf("RefreshHolding = %v, want nil", err)
	}
	if priced {
		t.Error("an unquoted symbol must not report a price")
	}
	if holding.LastPrice != 42 || holding.LastPriceUpdatedAt != nil {
		t.Errorf("holding was touched: price %v, stamp %v", holding.LastPrice, holding.LastPriceUpdatedAt)
	}
}

// Ineligible holdings must not reach the provider at all.
func TestRefreshHoldingSkipsIneligible(t *testing.T) {
	f := &fakeProvider{quotes: map[string]Quote{}}
	p := NewPricer(f)

	for _, h := range []models.Holding{
		{AssetType: models.AssetTypeStock, Symbol: "AAPL", Status: models.HoldingStatusClosed},
		{AssetType: models.AssetTypeOption, Symbol: "-AXP251121C390", Status: models.HoldingStatusOpen},
		{AssetType: models.AssetTypeStock, Status: models.HoldingStatusOpen},
	} {
		if _, err := p.RefreshHolding(context.Background(), nil, &h); err != nil {
			t.Errorf("RefreshHolding(%s) = %v, want nil", h.Symbol, err)
		}
	}
	if len(f.asked) != 0 {
		t.Errorf("provider was asked about %v, want nothing", f.asked)
	}
}
