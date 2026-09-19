package uploads

import "testing"

func qty(v float64) *float64 { return &v }

// mapAction carries the whole options model: which side of the market a trade
// lands on, and whether it opens or closes. Getting a sign backwards silently
// strands closes against lots they can never match, so the conventions are
// pinned here against the exact strings Fidelity exports.
func TestMapAction(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		quantity  *float64
		action    string
		effect    string
		direction string
	}{
		{
			name: "stock buy opens long",
			raw:  "YOU BOUGHT PROSHARES TR (UBER)", quantity: qty(100),
			action: actionBuy, effect: effectOpen, direction: directionLong,
		},
		{
			name: "stock sell closes long",
			raw:  "YOU SOLD PROSHARES TR (UBER)", quantity: qty(-100),
			action: actionSell, effect: effectClose, direction: directionLong,
		},
		{
			name: "treasury redemption closes long",
			raw:  "REDEMPTION PAYOUT (912797RS8)", quantity: qty(-50000),
			action: actionSell, effect: effectClose, direction: directionLong,
		},
		{
			name: "buying an option to open goes long",
			raw:  "YOU BOUGHT OPENING TRANSACTION CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $395 (100 SHS) (Margin)", quantity: qty(2),
			action: actionBuy, effect: effectOpen, direction: directionLong,
		},
		{
			name: "selling an option to close exits a long",
			raw:  "YOU SOLD CLOSING TRANSACTION CALL (CRCL) CIRCLE INTERNET JUN 20 25 $240 (100 SHS) (Margin)", quantity: qty(-3),
			action: actionSell, effect: effectClose, direction: directionLong,
		},
		{
			// The inversion: a sale that OPENS a position rather than closing one.
			name: "writing an option opens a short",
			raw:  "YOU SOLD OPENING TRANSACTION CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $390 (100 SHS) (Margin)", quantity: qty(-2),
			action: actionSell, effect: effectOpen, direction: directionShort,
		},
		{
			name: "buying an option to close covers a short",
			raw:  "YOU BOUGHT CLOSING TRANSACTION CALL (CRCL) CIRCLE INTERNET JUN 20 25 $230 (100 SHS) (Margin)", quantity: qty(3),
			action: actionBuy, effect: effectClose, direction: directionShort,
		},
		{
			// Fidelity records an expiration as the offsetting trade, so its sign
			// is opposite to the position it removes.
			name: "a long contract expiring is signed negative",
			raw:  "EXPIRED CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $395 as of 2025-11-21", quantity: qty(-2),
			action: actionExpired, effect: effectClose, direction: directionLong,
		},
		{
			name: "a written contract expiring is signed positive",
			raw:  "EXPIRED CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $390 as of 2025-11-21", quantity: qty(2),
			action: actionExpired, effect: effectClose, direction: directionShort,
		},
		{
			name: "a dividend touches no lot",
			raw:  "DIVIDEND RECEIVED (TLT)", quantity: nil,
			action: actionDividend, effect: "", direction: "",
		},
		{
			name: "an unmapped action passes through with no lot effect",
			raw:  "ELECTRONIC FUNDS TRANSFER RECEIVED", quantity: nil,
			action: "ELECTRONIC FUNDS TRANSFER RECEIVED", effect: "", direction: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action, effect, direction := mapAction(tc.raw, tc.quantity)
			if action != tc.action || effect != tc.effect || direction != tc.direction {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					action, effect, direction, tc.action, tc.effect, tc.direction)
			}
		})
	}
}
