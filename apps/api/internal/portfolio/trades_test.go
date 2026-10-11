package portfolio

import (
	"math"
	"testing"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func f64(v float64) *float64 { return &v }

// txnFill builds a fill on a fresh transaction. A nil price is an expiration.
func txnFill(holdingID uuid.UUID, date, effect, direction string, qty float64, price *float64, fees float64) models.TradeFill {
	q := qty
	txn := &models.Transaction{
		ID:        uuid.Must(uuid.NewV7()),
		HoldingID: &holdingID,
		Date:      day(date),
		Effect:    effect,
		Direction: direction,
		Quantity:  &q,
		Price:     price,
		Fees:      fees,
	}
	return models.TradeFill{TransactionID: txn.ID, Transaction: txn, Quantity: qty}
}

func TestComputeTradeMetrics(t *testing.T) {
	stock := uuid.Must(uuid.NewV7())
	call := uuid.Must(uuid.NewV7())
	open, close := models.EffectOpen, models.EffectClose
	long, short := models.DirectionLong, models.DirectionShort

	t.Run("scaling out of a long realizes each exit against the average", func(t *testing.T) {
		fills := []models.TradeFill{
			txnFill(stock, "2026-01-05", open, long, 100, f64(10), 1),
			txnFill(stock, "2026-01-20", close, long, 50, f64(12), 1),
			txnFill(stock, "2026-02-04", close, long, 50, f64(9), 1),
		}
		m := ComputeTradeMetrics(fills, map[uuid.UUID]TradeLeg{stock: {Multiplier: 1}})
		if !near(m.GrossPnL, 50) || !near(m.Fees, 3) || !near(m.NetPnL, 47) {
			t.Errorf("gross/fees/net = %v/%v/%v, want 50/3/47", m.GrossPnL, m.Fees, m.NetPnL)
		}
		if m.Status != models.TradeStatusClosed || !m.ClosedOn.Equal(day("2026-02-04")) {
			t.Errorf("status %s closed %v, want closed on 2026-02-04", m.Status, m.ClosedOn)
		}
		if !near(m.AvgEntryPrice, 10) || !near(m.AvgExitPrice, 10.5) || !near(m.EntryValue, 1000) {
			t.Errorf("entry/exit/value = %v/%v/%v", m.AvgEntryPrice, m.AvgExitPrice, m.EntryValue)
		}
		if m.UnrealizedPnL != nil {
			t.Errorf("a closed trade has nothing unrealized, got %v", *m.UnrealizedPnL)
		}
	})

	t.Run("scaling in averages the entries", func(t *testing.T) {
		fills := []models.TradeFill{
			txnFill(stock, "2026-01-05", open, long, 100, f64(10), 0),
			txnFill(stock, "2026-01-07", open, long, 100, f64(14), 0),
			txnFill(stock, "2026-01-09", close, long, 200, f64(13), 0),
		}
		m := ComputeTradeMetrics(fills, map[uuid.UUID]TradeLeg{stock: {Multiplier: 1}})
		if !near(m.GrossPnL, 200) || !near(m.AvgEntryPrice, 12) {
			t.Errorf("gross %v avg entry %v, want 200 and 12", m.GrossPnL, m.AvgEntryPrice)
		}
	})

	t.Run("a split between entry and exit is carried through", func(t *testing.T) {
		splits := Splits{{EffectiveDate: day("2026-03-01"), OldShares: 1, NewShares: 2}}
		fills := []models.TradeFill{
			txnFill(stock, "2026-02-01", open, long, 10, f64(100), 0),
			txnFill(stock, "2026-04-01", close, long, 20, f64(60), 0),
		}
		m := ComputeTradeMetrics(fills, map[uuid.UUID]TradeLeg{stock: {Multiplier: 1, Splits: splits}})
		if !near(m.GrossPnL, 200) || m.Status != models.TradeStatusClosed {
			t.Errorf("gross %v status %s, want 200 closed", m.GrossPnL, m.Status)
		}
		if !near(m.AvgEntryPrice, 50) {
			t.Errorf("avg entry %v, want 50 in today's shares", m.AvgEntryPrice)
		}
	})

	t.Run("a written option gains when bought back cheaper", func(t *testing.T) {
		fills := []models.TradeFill{
			txnFill(call, "2026-01-05", open, short, 2, f64(1.5), 0),
			txnFill(call, "2026-01-20", close, short, 2, f64(0.5), 0),
		}
		m := ComputeTradeMetrics(fills, map[uuid.UUID]TradeLeg{call: {Multiplier: 100}})
		if !near(m.GrossPnL, 200) || m.Direction != short {
			t.Errorf("gross %v direction %s, want 200 short", m.GrossPnL, m.Direction)
		}
	})

	t.Run("a covered call stays open until both legs are flat", func(t *testing.T) {
		legs := map[uuid.UUID]TradeLeg{
			stock: {Multiplier: 1, LastPrice: 55},
			call:  {Multiplier: 100, LastPrice: 1},
		}
		fills := []models.TradeFill{
			txnFill(stock, "2026-01-05", open, long, 100, f64(50), 0),
			txnFill(call, "2026-01-05", open, short, 1, f64(2), 0),
			txnFill(call, "2026-02-20", close, short, 1, nil, 0), // expired
		}
		m := ComputeTradeMetrics(fills, legs)
		if m.Status != models.TradeStatusOpen || !near(m.GrossPnL, 200) {
			t.Fatalf("status %s gross %v, want open with the 200 premium realized", m.Status, m.GrossPnL)
		}
		if m.UnrealizedPnL == nil || !near(*m.UnrealizedPnL, 500) {
			t.Errorf("unrealized %v, want 500 on the stock", m.UnrealizedPnL)
		}
		if m.Direction != long || !near(m.OpenQuantity, 100) {
			t.Errorf("first leg should lead: direction %s open %v", m.Direction, m.OpenQuantity)
		}

		fills = append(fills, txnFill(stock, "2026-03-02", close, long, 100, f64(52), 0))
		m = ComputeTradeMetrics(fills, legs)
		if m.Status != models.TradeStatusClosed || !near(m.GrossPnL, 400) {
			t.Errorf("status %s gross %v, want closed at 400", m.Status, m.GrossPnL)
		}
	})

	t.Run("an unpriced open leg leaves unrealized unknown", func(t *testing.T) {
		fills := []models.TradeFill{txnFill(stock, "2026-01-05", open, long, 10, f64(10), 0)}
		m := ComputeTradeMetrics(fills, map[uuid.UUID]TradeLeg{stock: {Multiplier: 1}})
		if m.UnrealizedPnL != nil {
			t.Errorf("unrealized %v, want nil", *m.UnrealizedPnL)
		}
	})

	t.Run("detached fills and over-attached exits don't count", func(t *testing.T) {
		detached := txnFill(stock, "2026-01-10", close, long, 10, f64(20), 0)
		detached.Detached = true
		fills := []models.TradeFill{
			txnFill(stock, "2026-01-05", open, long, 10, f64(10), 0),
			detached,
			txnFill(stock, "2026-01-12", close, long, 30, f64(12), 0),
		}
		m := ComputeTradeMetrics(fills, map[uuid.UUID]TradeLeg{stock: {Multiplier: 1}})
		if !near(m.GrossPnL, 20) || m.Status != models.TradeStatusClosed {
			t.Errorf("gross %v status %s, want only 10 shares closed for 20", m.GrossPnL, m.Status)
		}
	})
}

func TestApplyTradeMetrics(t *testing.T) {
	trade := models.Trade{PlannedRisk: f64(100)}
	opened, closed := day("2026-01-05"), day("2026-02-04")
	applyTradeMetrics(&trade, TradeMetrics{
		Status: models.TradeStatusClosed, OpenedOn: &opened, ClosedOn: &closed,
		EntryValue: 1000, GrossPnL: 50, Fees: 3, NetPnL: 47,
	})
	if trade.RMultiple == nil || !near(*trade.RMultiple, 0.47) {
		t.Errorf("R = %v, want 0.47", trade.RMultiple)
	}
	if trade.ReturnPercent == nil || !near(*trade.ReturnPercent, 4.7) {
		t.Errorf("return = %v, want 4.7", trade.ReturnPercent)
	}
	if trade.HoldingDays == nil || *trade.HoldingDays != 30 {
		t.Errorf("holding days = %v, want 30", trade.HoldingDays)
	}

	applyTradeMetrics(&trade, TradeMetrics{Status: models.TradeStatusOpen, OpenedOn: &opened, NetPnL: 20, EntryValue: 1000})
	if trade.RMultiple != nil || trade.HoldingDays != nil {
		t.Errorf("an open trade has no final R or holding period")
	}
}

func TestPlannedRisk(t *testing.T) {
	entry := models.Transaction{Price: f64(50)}
	if got := PlannedRisk(entry, 100, 47.5, 1); !near(got, 250) {
		t.Errorf("stock risk = %v, want 250", got)
	}
	if got := PlannedRisk(entry, 2, 49, 100); !near(got, 200) {
		t.Errorf("option risk = %v, want 200", got)
	}
}

// The allocation decides which sells close a journaled trade and which belong
// to shares held outside the journal, so the boundaries have to be exact.
func TestAllocateExits(t *testing.T) {
	tradeA, tradeB := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	id := func() uuid.UUID { return uuid.Must(uuid.NewV7()) }
	entry := func(trade uuid.UUID, date string, qty float64) allocEvent {
		return allocEvent{kind: allocEntry, tradeID: trade, txnID: id(), date: day(date), qty: qty}
	}
	closeOf := func(txn uuid.UUID, date string, qty float64) allocEvent {
		return allocEvent{kind: allocClose, txnID: txn, date: day(date), qty: qty}
	}

	t.Run("a sell larger than the trade only closes what the trade holds", func(t *testing.T) {
		sell := id()
		got := allocateExits([]allocEvent{entry(tradeA, "2026-01-05", 50), closeOf(sell, "2026-01-10", 150)}, nil)
		if !near(got[tradeTxn{tradeA, sell}], 50) || len(got) != 1 {
			t.Errorf("got %v, want 50 to trade A", got)
		}
	})

	t.Run("a sell before the trade opened isn't the trade's", func(t *testing.T) {
		sell := id()
		got := allocateExits([]allocEvent{closeOf(sell, "2026-01-04", 50), entry(tradeA, "2026-01-05", 50)}, nil)
		if len(got) != 0 {
			t.Errorf("got %v, want nothing", got)
		}
	})

	t.Run("a same-day entry opens before the sell that day", func(t *testing.T) {
		sell := id()
		got := allocateExits([]allocEvent{closeOf(sell, "2026-01-05", 50), entry(tradeA, "2026-01-05", 50)}, nil)
		if !near(got[tradeTxn{tradeA, sell}], 50) {
			t.Errorf("got %v, want the day trade closed", got)
		}
	})

	t.Run("concurrent trades are closed oldest first", func(t *testing.T) {
		sell := id()
		got := allocateExits([]allocEvent{
			entry(tradeB, "2026-01-06", 30),
			entry(tradeA, "2026-01-05", 30),
			closeOf(sell, "2026-01-10", 40),
		}, nil)
		if !near(got[tradeTxn{tradeA, sell}], 30) || !near(got[tradeTxn{tradeB, sell}], 10) {
			t.Errorf("got %v, want 30 to A then 10 to B", got)
		}
	})

	t.Run("a detached pair passes the sell to the next trade", func(t *testing.T) {
		sell := id()
		got := allocateExits([]allocEvent{
			entry(tradeA, "2026-01-05", 30),
			entry(tradeB, "2026-01-06", 30),
			closeOf(sell, "2026-01-10", 30),
		}, map[tradeTxn]bool{{tradeA, sell}: true})
		if _, ok := got[tradeTxn{tradeA, sell}]; ok || !near(got[tradeTxn{tradeB, sell}], 30) {
			t.Errorf("got %v, want all 30 to B", got)
		}
	})

	t.Run("a closed trade takes no later sells, and a manual exit counts", func(t *testing.T) {
		manual, later := id(), id()
		got := allocateExits([]allocEvent{
			entry(tradeA, "2026-01-05", 30),
			{kind: allocManualExit, tradeID: tradeA, txnID: manual, date: day("2026-01-08"), qty: 30},
			closeOf(later, "2026-01-10", 30),
		}, nil)
		if len(got) != 0 {
			t.Errorf("got %v, want nothing after the manual exit closed it", got)
		}
	})
}
