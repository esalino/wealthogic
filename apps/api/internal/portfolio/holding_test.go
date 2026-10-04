package portfolio

import (
	"testing"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
)

// A holding's status is derived, not stored, so the two ways a quantity reaches
// zero have to stay distinguishable: sold out of, versus never imported. Getting
// that wrong marks a whole un-imported portfolio as Closed.
func TestDeriveStatus(t *testing.T) {
	tests := []struct {
		name     string
		quantity float64
		closings int64
		want     string
	}{
		{
			name:     "a sale that zeroed the position closes it",
			quantity: 0, closings: 1, want: models.HoldingStatusClosed,
		},
		{
			name:     "no lots and no ledger is an un-imported history, not an exit",
			quantity: 0, closings: 0, want: models.HoldingStatusOpen,
		},
		{
			name:     "a partial sale leaves the position open",
			quantity: 50, closings: 1, want: models.HoldingStatusOpen,
		},
		{
			name:     "buying back in reopens a holding that had been closed",
			quantity: 100, closings: 2, want: models.HoldingStatusOpen,
		},
		{
			// Depleting lots subtracts floats, so exiting a fractional position
			// can leave a residue far below any real share count.
			name:     "float residue from depleting fractional lots still counts as flat",
			quantity: 1e-12, closings: 1, want: models.HoldingStatusClosed,
		},
		{
			name:     "a genuinely small position is not flat",
			quantity: 0.001, closings: 1, want: models.HoldingStatusOpen,
		},
		{
			// A written option nets negative: covering it is what closes it, and
			// an uncovered short is still an open position.
			name:     "an open written option is not closed",
			quantity: -2, closings: 0, want: models.HoldingStatusOpen,
		},
		{
			name:     "a residue on the short side is flat too",
			quantity: -1e-12, closings: 1, want: models.HoldingStatusClosed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveStatus(tt.quantity, tt.closings); got != tt.want {
				t.Errorf("DeriveStatus(%v, %d) = %s, want %s", tt.quantity, tt.closings, got, tt.want)
			}
		})
	}
}

// A new price has to carry the market value and unrealized gain with it, or the
// portfolio would show a fresh price against a stale value. The position itself
// comes from the ledger and must survive untouched.
func TestApplyPriceRepricesWithoutDisturbingThePosition(t *testing.T) {
	holding := models.Holding{
		Quantity:       10,
		CostBasisTotal: 1000,
		LastPrice:      100,
	}
	asOf := mustTime(t, "2026-10-02T20:00:00Z")

	ApplyPrice(&holding, 150, asOf)

	if holding.CurrentValue != 1500 {
		t.Errorf("current value = %v, want 1500", holding.CurrentValue)
	}
	if holding.GainUnrealizedAmount != 500 {
		t.Errorf("unrealized gain = %v, want 500", holding.GainUnrealizedAmount)
	}
	if holding.GainUnrealizedPercent != 50 {
		t.Errorf("unrealized percent = %v, want 50", holding.GainUnrealizedPercent)
	}
	if holding.LastPriceUpdatedAt == nil || !holding.LastPriceUpdatedAt.Equal(asOf) {
		t.Errorf("stamp = %v, want %v", holding.LastPriceUpdatedAt, asOf)
	}
	if holding.Quantity != 10 || holding.CostBasisTotal != 1000 {
		t.Errorf("the position moved: quantity %v, basis %v", holding.Quantity, holding.CostBasisTotal)
	}
}

// An option's price is quoted per share while the contract covers a hundred of
// them, so the multiplier has to survive a reprice.
func TestApplyPriceScalesByContractMultiplier(t *testing.T) {
	holding := models.Holding{Quantity: 2, ContractMultiplier: 100, CostBasisTotal: 400}

	ApplyPrice(&holding, 3, mustTime(t, "2026-10-02T20:00:00Z"))

	if holding.CurrentValue != 600 {
		t.Errorf("current value = %v, want 600 (2 contracts x 100 shares x $3)", holding.CurrentValue)
	}
}

// mustTime parses a timestamp the test wrote itself, so a bad literal is a bug
// in the test rather than a failure to report.
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad timestamp %q in test: %v", s, err)
	}
	return parsed
}
