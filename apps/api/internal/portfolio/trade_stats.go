package portfolio

import (
	"sort"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
)

// TradeStats is how a set of closed trades performed - usually one strategy's.
// Ratios that would divide by zero are nil rather than zero, so "no losses yet"
// doesn't read as a profit factor of 0.
type TradeStats struct {
	StrategyID   *uuid.UUID `json:"strategy_id"`
	StrategyName string     `json:"strategy_name"`

	Trades  int      `json:"trades"`
	Wins    int      `json:"wins"`
	Losses  int      `json:"losses"`
	WinRate *float64 `json:"win_rate"` // percent

	NetPnL      float64  `json:"net_pnl"`
	AvgWin      *float64 `json:"avg_win"`
	AvgLoss     *float64 `json:"avg_loss"` // negative
	LargestWin  float64  `json:"largest_win"`
	LargestLoss float64  `json:"largest_loss"`

	// ProfitFactor is gross winnings over gross losses; Expectancy is the
	// average net result per trade.
	ProfitFactor *float64 `json:"profit_factor"`
	Expectancy   *float64 `json:"expectancy"`

	// AvgR is averaged over the trades that had a planned risk (RTrades of
	// them), since a trade without a stop has no R.
	AvgR    *float64 `json:"avg_r"`
	RTrades int      `json:"r_trades"`

	AvgHoldingDays *float64 `json:"avg_holding_days"`

	// MaxDrawdown is the largest peak-to-trough fall of cumulative net P&L,
	// taking trades in the order they closed. Positive, in dollars.
	MaxDrawdown float64 `json:"max_drawdown"`

	// RuleAdherence is the percent of answered rule checks that were met,
	// across these trades. Nil when none were answered.
	RuleAdherence *float64 `json:"rule_adherence"`
} // @name TradeStats

// SummarizeTrades computes stats over the closed trades given; open ones are
// skipped, since their result isn't final. RuleChecks must be loaded for
// RuleAdherence to count them.
func SummarizeTrades(trades []models.Trade) TradeStats {
	var closed []models.Trade
	for _, t := range trades {
		if t.Status == models.TradeStatusClosed {
			closed = append(closed, t)
		}
	}
	sort.SliceStable(closed, func(i, j int) bool {
		a, b := closed[i].ClosedOn, closed[j].ClosedOn
		if a != nil && b != nil && !a.Equal(*b) {
			return a.Before(*b)
		}
		return uuidLess(closed[i].ID, closed[j].ID)
	})

	var (
		s                    TradeStats
		won, lost            float64
		rSum                 float64
		daysSum              float64
		daysCount            int
		cumulative, peak     float64
		checksMet, checksAll int
	)
	s.Trades = len(closed)
	for _, t := range closed {
		net := t.NetPnL
		s.NetPnL += net
		switch {
		case net > 0:
			s.Wins++
			won += net
			if net > s.LargestWin {
				s.LargestWin = net
			}
		case net < 0:
			s.Losses++
			lost += net
			if net < s.LargestLoss {
				s.LargestLoss = net
			}
		}
		if t.RMultiple != nil {
			rSum += *t.RMultiple
			s.RTrades++
		}
		if t.HoldingDays != nil {
			daysSum += float64(*t.HoldingDays)
			daysCount++
		}

		cumulative += net
		if cumulative > peak {
			peak = cumulative
		}
		if peak-cumulative > s.MaxDrawdown {
			s.MaxDrawdown = peak - cumulative
		}

		for _, c := range t.RuleChecks {
			if c.Met == nil {
				continue
			}
			checksAll++
			if *c.Met {
				checksMet++
			}
		}
	}

	ratio := func(num, den float64) *float64 {
		if den == 0 {
			return nil
		}
		v := num / den
		return &v
	}
	if s.Trades > 0 {
		s.WinRate = ratio(float64(s.Wins)*100, float64(s.Trades))
		s.Expectancy = ratio(s.NetPnL, float64(s.Trades))
	}
	s.AvgWin = ratio(won, float64(s.Wins))
	s.AvgLoss = ratio(lost, float64(s.Losses))
	s.ProfitFactor = ratio(won, -lost)
	s.AvgR = ratio(rSum, float64(s.RTrades))
	s.AvgHoldingDays = ratio(daysSum, float64(daysCount))
	s.RuleAdherence = ratio(float64(checksMet)*100, float64(checksAll))
	return s
}
