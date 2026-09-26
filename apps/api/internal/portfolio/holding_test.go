package portfolio

import (
	"testing"

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
