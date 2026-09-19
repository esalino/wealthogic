package tax

import "testing"

func TestCapitalPosition(t *testing.T) {
	tests := []struct {
		name                                   string
		short, long                            float64
		net, offset, taxable, lossCarryforward float64
	}{
		{
			name:  "gains in both periods add up, nothing offsets",
			short: 500, long: 1000,
			net: 1500, offset: 0, taxable: 1500,
		},
		{
			// Step two: a loss in one period absorbs the other's gain.
			name:  "a short-term loss offsets a long-term gain",
			short: -400, long: 1000,
			net: 600, offset: 400, taxable: 600,
		},
		{
			name:  "a long-term loss offsets a short-term gain",
			short: 1000, long: -400,
			net: 600, offset: 400, taxable: 600,
		},
		{
			// The loss is larger than the gain it offsets, so the gain is fully
			// absorbed and the remainder carries forward.
			name:  "a loss bigger than the gain leaves a carryforward",
			short: 1000, long: -2500,
			net: -1500, offset: 1000, taxable: 0, lossCarryforward: 1500,
		},
		{
			name:  "losses in both periods offset nothing and all carry forward",
			short: -300, long: -700,
			net: -1000, offset: 0, taxable: 0, lossCarryforward: 1000,
		},
		{
			name:  "an exactly cancelling pair is taxable on nothing",
			short: 800, long: -800,
			net: 0, offset: 800, taxable: 0,
		},
		{
			name:  "one period empty",
			short: 0, long: 1200,
			net: 1200, offset: 0, taxable: 1200,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := CapitalPosition{ShortTerm: tc.short, LongTerm: tc.long}
			if got := c.Net(); got != tc.net {
				t.Errorf("Net() = %v, want %v", got, tc.net)
			}
			if got := c.Offset(); got != tc.offset {
				t.Errorf("Offset() = %v, want %v", got, tc.offset)
			}
			if got := c.Taxable(); got != tc.taxable {
				t.Errorf("Taxable() = %v, want %v", got, tc.taxable)
			}
			if got := c.LossCarryforward(); got != tc.lossCarryforward {
				t.Errorf("LossCarryforward() = %v, want %v", got, tc.lossCarryforward)
			}
		})
	}
}

// The defect this netting exists to prevent: a losing year of trading must not
// erase interest income, which is taxable in full regardless.
func TestCapitalLossDoesNotReduceOrdinaryIncome(t *testing.T) {
	capital := CapitalPosition{ShortTerm: 4383.00, LongTerm: -12446.12}
	ordinary := 7363.43

	total := capital.Taxable() + ordinary
	if total != ordinary {
		t.Errorf("taxable total = %.2f, want %.2f - the capital loss must not touch ordinary income", total, ordinary)
	}
	if got := capital.LossCarryforward(); got < 8063.11 || got > 8063.13 {
		t.Errorf("carryforward = %.2f, want 8063.12", got)
	}
	// Naively summing everything is what produced the wrong number.
	if naive := capital.Net() + ordinary; naive >= total {
		t.Errorf("sanity: the naive total %.2f should be lower than the correct %.2f", naive, total)
	}
}
