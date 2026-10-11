package portfolio

import (
	"math"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// Every quantity that crosses a split goes through Factor, so the ex-date
// boundary and the compounding of several splits have to be exactly right.
func TestSplitsFactor(t *testing.T) {
	splits := Splits{
		{EffectiveDate: day("2024-06-04"), OldShares: 8, NewShares: 1}, // 1-for-8 reverse
		{EffectiveDate: day("2025-03-01"), OldShares: 1, NewShares: 4}, // 4-for-1 forward
	}

	tests := []struct {
		name string
		date string
		want float64
	}{
		{"before both splits compounds them", "2024-01-15", 0.5},
		{"the day before the ex-date is still pre-split", "2024-06-03", 0.5},
		{"a trade on the ex-date is already post-split", "2024-06-04", 4},
		{"between the splits only the later one applies", "2024-12-31", 4},
		{"after every split nothing applies", "2025-03-01", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splits.Factor(day(tt.date)); math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("Factor(%s) = %v, want %v", tt.date, got, tt.want)
			}
		})
	}
}

func TestSplitsRoundTrip(t *testing.T) {
	splits := Splits{{EffectiveDate: day("2024-06-04"), OldShares: 8, NewShares: 1}}
	bought := day("2024-01-15")

	// 2000 pre-split shares are 250 today, and 250 today are 2000 as traded.
	if got := splits.ToCurrent(2000, bought); got != 250 {
		t.Errorf("ToCurrent = %v, want 250", got)
	}
	if got := splits.FromCurrent(250, bought); got != 2000 {
		t.Errorf("FromCurrent = %v, want 2000", got)
	}
}

func TestNoSplitsIsIdentity(t *testing.T) {
	var splits Splits
	if got := splits.Factor(day("2020-01-01")); got != 1 {
		t.Errorf("Factor with no splits = %v, want 1", got)
	}
}
