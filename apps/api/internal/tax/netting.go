package tax

// Capital gains net against each other before anything is owed, and the order
// matters: like against like first, then whatever loss is left over against the
// other side. Ordinary income - interest and dividends - sits outside that
// entirely and cannot be reduced by a capital loss.
//
// Summing every taxable amount together, as a single total would, quietly lets
// a losing year of trading erase interest income that is fully taxable.

// CapitalPosition is a year's capital gains for one jurisdiction, already summed
// within each holding period. Either may be negative.
type CapitalPosition struct {
	ShortTerm float64
	LongTerm  float64
}

// Net is what remains after both netting steps.
//
// Step one nets within each holding period, which is what summing the events
// into ShortTerm and LongTerm already did. Step two applies any remaining loss
// in one period against the other's gain - and since a loss is a negative
// number, that second step is just the sum. The steps are named because the
// intermediate figures are what a taxpayer checks, not because they need
// separate arithmetic.
func (c CapitalPosition) Net() float64 {
	return c.ShortTerm + c.LongTerm
}

// Offset is how much of one period's loss was absorbed by the other's gain -
// the size of step two, reported so a summary can show the rule at work.
func (c CapitalPosition) Offset() float64 {
	// Only when the two sides disagree in sign does either absorb the other.
	if c.ShortTerm >= 0 && c.LongTerm >= 0 {
		return 0
	}
	if c.ShortTerm <= 0 && c.LongTerm <= 0 {
		return 0
	}
	loss, gain := c.ShortTerm, c.LongTerm
	if loss > 0 {
		loss, gain = c.LongTerm, c.ShortTerm
	}
	// The absorbed amount is whichever is smaller in magnitude.
	if -loss < gain {
		return -loss
	}
	return gain
}

// Taxable is the part of the net that adds to taxable income. A net capital
// loss adds nothing: it doesn't reduce other income here, it carries forward.
func (c CapitalPosition) Taxable() float64 {
	if net := c.Net(); net > 0 {
		return net
	}
	return 0
}

// LossCarryforward is the net loss this year leaves behind, as a positive
// number. Reported rather than applied - carryforward across tax years isn't
// modeled, and neither is the limited deduction against ordinary income some
// jurisdictions allow.
func (c CapitalPosition) LossCarryforward() float64 {
	if net := c.Net(); net < 0 {
		return -net
	}
	return 0
}
