package portfolio

import (
	"bytes"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Trades are a journal over the transaction ledger (see models.Trade). This
// file keeps them in step with it: attaching the closes that exit a trade, and
// recomputing what each trade made.
//
// The math is average-cost per leg, in today's shares. Every entry adds its
// units and price-only value to the leg; every exit takes units off at the
// leg's average and realizes the difference. Commission and fees are summed
// apart and come off the gross at the end, so gross and net both fall out.

// TradeLeg is what the trade math needs to know about one holding a trade has
// fills in.
type TradeLeg struct {
	Multiplier float64
	Splits     Splits
	LastPrice  float64
}

// TradeMetrics is what a trade's fills add up to.
type TradeMetrics struct {
	Direction string
	Status    string
	OpenedOn  *time.Time
	ClosedOn  *time.Time

	OpenQuantity  float64
	AvgEntryPrice float64
	AvgExitPrice  float64
	EntryValue    float64
	GrossPnL      float64
	Fees          float64
	NetPnL        float64

	// UnrealizedPnL values what's still open at each leg's LastPrice. Nil when
	// nothing is open, or when an open leg has no price to value it at.
	UnrealizedPnL *float64
}

// legKey is one side of the market in one holding - the unit an exit closes
// against, as it is for tax lots.
type legKey struct {
	holdingID uuid.UUID
	direction string
}

func normDirection(direction string) string {
	if direction == "" {
		return models.DirectionLong
	}
	return direction
}

func uuidLess(a, b uuid.UUID) bool { return bytes.Compare(a[:], b[:]) < 0 }

// activeFills is the fills that count toward a trade's figures, oldest first.
// On the same day entries come before exits: Fidelity's intraday order isn't
// reliable, and a trade has to be open before anything can close it.
func activeFills(fills []models.TradeFill, legs map[uuid.UUID]TradeLeg) []models.TradeFill {
	var active []models.TradeFill
	for _, f := range fills {
		if f.Detached || f.Transaction == nil || f.Transaction.HoldingID == nil {
			continue
		}
		if _, ok := legs[*f.Transaction.HoldingID]; !ok {
			continue
		}
		active = append(active, f)
	}
	sort.SliceStable(active, func(i, j int) bool {
		a, b := active[i].Transaction, active[j].Transaction
		if !a.Date.Equal(b.Date) {
			return a.Date.Before(b.Date)
		}
		if (a.Effect == models.EffectOpen) != (b.Effect == models.EffectOpen) {
			return a.Effect == models.EffectOpen
		}
		return uuidLess(a.ID, b.ID)
	})
	return active
}

// ComputeTradeMetrics adds up a trade's fills. Each fill must carry its
// Transaction, and legs must hold every holding the fills are in; fills
// without either are left out.
func ComputeTradeMetrics(fills []models.TradeFill, legs map[uuid.UUID]TradeLeg) TradeMetrics {
	type legState struct {
		open  float64 // units still open, in today's shares
		basis float64 // price-only value of those units
	}
	var (
		m        TradeMetrics
		states   = map[legKey]*legState{}
		primary  *legKey
		hadExit  bool
		lastExit time.Time

		entryQty, entryValue float64 // first leg only, for its average prices
		exitQty, exitValue   float64
	)

	for _, f := range activeFills(fills, legs) {
		txn := f.Transaction
		leg := legs[*txn.HoldingID]
		multiplier := leg.Multiplier
		if multiplier <= 0 {
			multiplier = 1
		}
		key := legKey{*txn.HoldingID, normDirection(txn.Direction)}
		st, ok := states[key]
		if !ok {
			st = &legState{}
			states[key] = st
		}
		date := txn.Date
		today := leg.Splits.ToCurrent(f.Quantity, date)

		if txn.Effect == models.EffectOpen {
			if primary == nil {
				k := key
				primary = &k
				m.Direction = key.direction
				d := date
				m.OpenedOn = &d
			}
			value := tradeValue(*txn, f.Quantity, multiplier)
			st.open += today
			st.basis += value
			m.EntryValue += value
			m.Fees += feeShare(*txn, f.Quantity)
			if key == *primary {
				entryQty += today
				entryValue += value
			}
			continue
		}

		// An exit can only close what the leg has open. Anything beyond that
		// was attached by hand to a trade that didn't hold it, and is ignored
		// rather than inventing a position on the other side.
		take := math.Min(today, st.open)
		if take <= quantityEpsilon {
			continue
		}
		traded := f.Quantity * take / today
		value := tradeValue(*txn, traded, multiplier)
		portion := st.basis * take / st.open
		if key.direction == models.DirectionShort {
			// Written: the premium came in at entry, buying back is the cost.
			m.GrossPnL += portion - value
		} else {
			m.GrossPnL += value - portion
		}
		m.Fees += feeShare(*txn, traded)
		st.open -= take
		st.basis -= portion
		if primary != nil && key == *primary {
			exitQty += take
			exitValue += value
		}
		hadExit = true
		lastExit = date
	}

	m.NetPnL = m.GrossPnL - m.Fees

	flat := true
	for _, st := range states {
		if st.open > quantityEpsilon {
			flat = false
		}
	}
	m.Status = models.TradeStatusOpen
	if hadExit && flat {
		m.Status = models.TradeStatusClosed
		d := lastExit
		m.ClosedOn = &d
	}

	if primary != nil {
		multiplier := legs[primary.holdingID].Multiplier
		if multiplier <= 0 {
			multiplier = 1
		}
		if st := states[*primary]; st.open > quantityEpsilon {
			m.OpenQuantity = st.open
		}
		if entryQty > 0 {
			m.AvgEntryPrice = entryValue / (entryQty * multiplier)
		}
		if exitQty > 0 {
			m.AvgExitPrice = exitValue / (exitQty * multiplier)
		}
	}

	// Value whatever is still open. One unpriced leg makes the whole figure
	// unknown, rather than quietly counting it as zero.
	var unrealized float64
	anyOpen, priced := false, true
	for key, st := range states {
		if st.open <= quantityEpsilon {
			continue
		}
		anyOpen = true
		leg := legs[key.holdingID]
		if leg.LastPrice <= 0 {
			priced = false
			break
		}
		multiplier := leg.Multiplier
		if multiplier <= 0 {
			multiplier = 1
		}
		market := st.open * leg.LastPrice * multiplier
		if key.direction == models.DirectionShort {
			unrealized += st.basis - market
		} else {
			unrealized += market - st.basis
		}
	}
	if anyOpen && priced {
		m.UnrealizedPnL = &unrealized
	}
	return m
}

// applyTradeMetrics copies computed figures onto a trade, along with the ones
// that follow from them and the trade's plan.
func applyTradeMetrics(trade *models.Trade, m TradeMetrics) {
	if m.Direction != "" {
		trade.Direction = m.Direction
	}
	trade.Status = m.Status
	trade.OpenedOn = m.OpenedOn
	trade.ClosedOn = m.ClosedOn
	trade.OpenQuantity = m.OpenQuantity
	trade.AvgEntryPrice = m.AvgEntryPrice
	trade.AvgExitPrice = m.AvgExitPrice
	trade.EntryValue = m.EntryValue
	trade.GrossPnL = m.GrossPnL
	trade.Fees = m.Fees
	trade.NetPnL = m.NetPnL

	trade.ReturnPercent = nil
	if m.EntryValue > 0 {
		pct := m.NetPnL / m.EntryValue * 100
		trade.ReturnPercent = &pct
	}

	trade.RMultiple, trade.HoldingDays = nil, nil
	if m.Status == models.TradeStatusClosed {
		if trade.PlannedRisk != nil && *trade.PlannedRisk > 0 {
			r := m.NetPnL / *trade.PlannedRisk
			trade.RMultiple = &r
		}
		if m.OpenedOn != nil && m.ClosedOn != nil {
			days := int(m.ClosedOn.Sub(*m.OpenedOn).Hours() / 24)
			trade.HoldingDays = &days
		}
	}
}

// PlannedRisk is the dollars lost if a single-leg trade is stopped out:
// the distance from entry to stop, on every unit the entry bought, scaled by
// the contract multiplier.
func PlannedRisk(entry models.Transaction, quantity, stop, multiplier float64) float64 {
	if multiplier <= 0 {
		multiplier = 1
	}
	return math.Abs(unitPrice(entry)-stop) * quantity * multiplier
}

// loadTradeLegs loads the holding details the trade math needs for every
// holding the fills touch. withPrices sets LastPrice for valuing open legs.
func loadTradeLegs(tx *gorm.DB, fills []models.TradeFill, withPrices bool) (map[uuid.UUID]TradeLeg, error) {
	legs := map[uuid.UUID]TradeLeg{}
	for _, f := range fills {
		if f.Transaction == nil || f.Transaction.HoldingID == nil {
			continue
		}
		id := *f.Transaction.HoldingID
		if _, ok := legs[id]; ok {
			continue
		}
		var holding models.Holding
		if err := tx.First(&holding, "id = ?", id).Error; err != nil {
			return nil, err
		}
		splits, err := LoadSplits(tx, id)
		if err != nil {
			return nil, err
		}
		leg := TradeLeg{Multiplier: holding.Multiplier(), Splits: splits}
		if withPrices {
			leg.LastPrice = holding.LastPrice
		}
		legs[id] = leg
	}
	return legs, nil
}

// RecomputeTrade recomputes a trade's cached figures from its fills and saves
// them. Callers that change the plan (the stop or the planned risk) call it
// too, since R depends on both.
func RecomputeTrade(tx *gorm.DB, tradeID uuid.UUID) error {
	var trade models.Trade
	if err := tx.First(&trade, "id = ?", tradeID).Error; err != nil {
		return err
	}
	var fills []models.TradeFill
	if err := tx.Preload("Transaction").Where("trade_id = ?", tradeID).Find(&fills).Error; err != nil {
		return err
	}
	legs, err := loadTradeLegs(tx, fills, false)
	if err != nil {
		return err
	}
	applyTradeMetrics(&trade, ComputeTradeMetrics(fills, legs))
	return tx.Model(&trade).Select(
		"direction", "status", "opened_on", "closed_on", "open_quantity",
		"avg_entry_price", "avg_exit_price", "entry_value", "gross_pnl", "fees",
		"net_pnl", "return_percent", "r_multiple", "holding_days",
	).Updates(&trade).Error
}

// ValueOpenTrades sets UnrealizedPnL on the open trades given, at their
// holdings' current prices. The trades' fills must be loaded with their
// transactions.
func ValueOpenTrades(tx *gorm.DB, trades []models.Trade) error {
	for i := range trades {
		if trades[i].Status != models.TradeStatusOpen {
			continue
		}
		legs, err := loadTradeLegs(tx, trades[i].Fills, true)
		if err != nil {
			return err
		}
		trades[i].UnrealizedPnL = ComputeTradeMetrics(trades[i].Fills, legs).UnrealizedPnL
	}
	return nil
}

// allocEvent is something that moves a trade's open quantity on one leg, in
// today's shares. tradeID is unset for a close, which isn't anyone's yet.
type allocEvent struct {
	kind    int // allocEntry, allocManualExit, allocClose
	tradeID uuid.UUID
	txnID   uuid.UUID
	date    time.Time
	qty     float64
}

const (
	allocEntry = iota
	allocManualExit
	allocClose
)

type tradeTxn struct{ trade, txn uuid.UUID }

// allocateExits decides which trades the closing trades on one leg exit, and
// by how much. It replays the leg oldest first: entries open quantity on their
// trade, manual exits close it, and each close is handed out to the trades with
// quantity open at that moment, oldest trade first, until it runs out. What's
// left over belongs to no trade - shares held outside the journal.
//
// skip holds the trade/transaction pairs the user already decided on: a fill
// they attached by hand, or one they detached. Those are never auto-filled.
func allocateExits(events []allocEvent, skip map[tradeTxn]bool) map[tradeTxn]float64 {
	sort.SliceStable(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if !a.date.Equal(b.date) {
			return a.date.Before(b.date)
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return uuidLess(a.txnID, b.txnID)
	})

	// Trades in the order they first opened on this leg, which is the order
	// closes are offered to them.
	var order []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, e := range events {
		if e.kind == allocEntry && !seen[e.tradeID] {
			seen[e.tradeID] = true
			order = append(order, e.tradeID)
		}
	}

	open := map[uuid.UUID]float64{}
	out := map[tradeTxn]float64{}
	for _, e := range events {
		switch e.kind {
		case allocEntry:
			open[e.tradeID] += e.qty
		case allocManualExit:
			open[e.tradeID] = math.Max(0, open[e.tradeID]-e.qty)
		case allocClose:
			remaining := e.qty
			for _, t := range order {
				if remaining <= quantityEpsilon {
					break
				}
				if skip[tradeTxn{t, e.txnID}] {
					continue
				}
				take := math.Min(open[t], remaining)
				if take <= quantityEpsilon {
					continue
				}
				out[tradeTxn{t, e.txnID}] += take
				open[t] -= take
				remaining -= take
			}
		}
	}
	return out
}

// exitReasonFor is the exit reason a close implies on its own, so an option
// that expired doesn't need the user to say so.
func exitReasonFor(txn models.Transaction) string {
	switch {
	case strings.EqualFold(txn.Action, "Expired"):
		return models.ExitReasonExpired
	case strings.Contains(strings.ToUpper(txn.Action), "ASSIGNED"):
		return models.ExitReasonAssigned
	}
	return ""
}

// SyncHoldingTrades brings every trade with a fill in this holding up to date
// with the ledger: fills on deleted transactions are dropped, the closes on
// each leg are re-allocated to the trades that had shares open, and each
// affected trade's figures are recomputed.
//
// It runs at the end of every holding recompute, so a trade follows whatever
// changed the ledger - a hand-entered sell, an import, an edit, a split. It is
// idempotent: auto fills are matched by trade and transaction and updated in
// place, so an exit reason the user recorded on one survives the replay.
func SyncHoldingTrades(tx *gorm.DB, holdingID uuid.UUID) error {
	// The join is raw, so it reaches soft-deleted transactions too - which is
	// how fills on them are found and cleaned up.
	var tradeIDs []uuid.UUID
	if err := tx.Model(&models.TradeFill{}).
		Joins("JOIN transactions ON transactions.id = trade_fills.transaction_id").
		Where("transactions.holding_id = ?", holdingID).
		Distinct("trade_fills.trade_id").Pluck("trade_fills.trade_id", &tradeIDs).Error; err != nil {
		return err
	}
	if len(tradeIDs) == 0 {
		return nil
	}

	deleted := tx.Unscoped().Model(&models.Transaction{}).Select("id").
		Where("holding_id = ? AND deleted_at IS NOT NULL", holdingID)
	if err := tx.Where("trade_id IN ? AND transaction_id IN (?)", tradeIDs, deleted).
		Delete(&models.TradeFill{}).Error; err != nil {
		return err
	}

	var fills []models.TradeFill
	if err := tx.Preload("Transaction").Where("trade_id IN ?", tradeIDs).Find(&fills).Error; err != nil {
		return err
	}
	splits, err := LoadSplits(tx, holdingID)
	if err != nil {
		return err
	}

	// Sort this holding's fills into legs. A leg is also per account: a trade
	// lives in one account, and a sell in another can't close it.
	type accountLeg struct {
		accountID uuid.UUID
		direction string
	}
	type legFills struct {
		events []allocEvent
		auto   []models.TradeFill // existing auto fills, live or detached
		skip   map[tradeTxn]bool
		used   map[uuid.UUID]float64 // today's shares of a close already assigned by hand
	}
	legs := map[accountLeg]*legFills{}
	legFor := func(k accountLeg) *legFills {
		l, ok := legs[k]
		if !ok {
			l = &legFills{skip: map[tradeTxn]bool{}, used: map[uuid.UUID]float64{}}
			legs[k] = l
		}
		return l
	}
	for _, f := range fills {
		txn := f.Transaction
		if txn == nil || txn.HoldingID == nil || *txn.HoldingID != holdingID {
			continue
		}
		l := legFor(accountLeg{txn.AccountID, normDirection(txn.Direction)})
		pair := tradeTxn{f.TradeID, f.TransactionID}
		today := splits.ToCurrent(f.Quantity, txn.Date)

		switch {
		case f.Source == models.FillSourceAuto:
			l.auto = append(l.auto, f)
			if f.Detached {
				l.skip[pair] = true
			}
		case f.Detached:
			l.skip[pair] = true
		case txn.Effect == models.EffectOpen:
			l.events = append(l.events, allocEvent{kind: allocEntry, tradeID: f.TradeID, txnID: txn.ID, date: txn.Date, qty: today})
		default:
			l.skip[pair] = true
			l.used[txn.ID] += today
			l.events = append(l.events, allocEvent{kind: allocManualExit, tradeID: f.TradeID, txnID: txn.ID, date: txn.Date, qty: today})
		}
	}

	for key, l := range legs {
		// The closes on this leg, less whatever has been assigned by hand.
		var closes []models.Transaction
		if err := tx.Where("holding_id = ? AND account_id = ? AND effect = ? AND quantity IS NOT NULL",
			holdingID, key.accountID, models.EffectClose).Find(&closes).Error; err != nil {
			return err
		}
		byID := map[uuid.UUID]models.Transaction{}
		for _, c := range closes {
			if normDirection(c.Direction) != key.direction {
				continue
			}
			byID[c.ID] = c
			available := splits.ToCurrent(absQuantity(c), c.Date) - l.used[c.ID]
			if available > quantityEpsilon {
				l.events = append(l.events, allocEvent{kind: allocClose, txnID: c.ID, date: c.Date, qty: available})
			}
		}
		want := allocateExits(l.events, l.skip)

		// Reconcile with the auto fills already there: update the ones still
		// wanted, drop the rest, then add what's new. Detached rows stay put.
		for _, f := range l.auto {
			if f.Detached {
				continue
			}
			pair := tradeTxn{f.TradeID, f.TransactionID}
			qty, ok := want[pair]
			if !ok {
				if err := tx.Delete(&models.TradeFill{}, "id = ?", f.ID).Error; err != nil {
					return err
				}
				continue
			}
			delete(want, pair)
			traded := splits.FromCurrent(qty, f.Transaction.Date)
			if math.Abs(traded-f.Quantity) > quantityEpsilon {
				if err := tx.Model(&models.TradeFill{}).Where("id = ?", f.ID).
					Update("quantity", traded).Error; err != nil {
					return err
				}
			}
		}
		for pair, qty := range want {
			txn := byID[pair.txn]
			fill := models.TradeFill{
				TradeID:       pair.trade,
				TransactionID: pair.txn,
				Quantity:      splits.FromCurrent(qty, txn.Date),
				Source:        models.FillSourceAuto,
				ExitReason:    exitReasonFor(txn),
			}
			if err := tx.Create(&fill).Error; err != nil {
				return err
			}
		}
	}

	for _, id := range tradeIDs {
		if err := RecomputeTrade(tx, id); err != nil {
			return err
		}
	}
	return nil
}
