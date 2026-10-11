package handlers

import (
	"bytes"
	"errors"
	"math"
	"net/http"
	"slices"
	"sort"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Trades are started by hand from an opening transaction and then kept in step
// with the ledger by portfolio.SyncHoldingTrades, which attaches the closes
// that exit them. These handlers own the parts only the user can say: the
// plan, the strategy, which fills belong, and how the rules were followed.

type TradeHandler interface {
	GetTrades(c *gin.Context)
	GetTrade(c *gin.Context)
	CreateTrade(c *gin.Context)
	UpdateTrade(c *gin.Context)
	DeleteTrade(c *gin.Context)
	AddFill(c *gin.Context)
	UpdateFill(c *gin.Context)
	RemoveFill(c *gin.Context)
	UpdateRuleChecks(c *gin.Context)
	GetCandidates(c *gin.Context)
}

type tradeHandler struct {
	db *gorm.DB
}

func NewTradeHandler(db *gorm.DB) TradeHandler {
	return &tradeHandler{db: db}
}

// errTradeInput is a request the trade can't accept, reported as a 400 with
// its own message.
type errTradeInput struct{ msg string }

func (e errTradeInput) Error() string { return e.msg }

// fillEpsilon absorbs float residue when comparing share counts.
const fillEpsilon = 1e-9

type tradeList []models.Trade // @name Trades

// withTradeDetail loads what a trade response carries: its fills with their
// transactions, its rule checks, and its strategy - even a deleted one, which
// still names the trades taken under it.
func withTradeDetail(q *gorm.DB) *gorm.DB {
	return q.Preload("Fills.Transaction").
		Preload("RuleChecks", preloadRules).
		Preload("Strategy", func(db *gorm.DB) *gorm.DB { return db.Unscoped() })
}

// finishTrades puts each trade's fills in date order and values what's open.
func finishTrades(db *gorm.DB, trades []models.Trade) error {
	for i := range trades {
		fills := trades[i].Fills
		sort.SliceStable(fills, func(a, b int) bool {
			ta, tb := fills[a].Transaction, fills[b].Transaction
			if ta == nil || tb == nil {
				return tb == nil && ta != nil
			}
			if !ta.Date.Equal(tb.Date) {
				return ta.Date.Before(tb.Date)
			}
			return bytes.Compare(ta.ID[:], tb.ID[:]) < 0
		})
	}
	return portfolio.ValueOpenTrades(db, trades)
}

func (h *tradeHandler) loadTrade(id uuid.UUID) (*models.Trade, error) {
	var trade models.Trade
	if err := withTradeDetail(h.db).First(&trade, "id = ?", id).Error; err != nil {
		return nil, err
	}
	trades := []models.Trade{trade}
	if err := finishTrades(h.db, trades); err != nil {
		return nil, err
	}
	return &trades[0], nil
}

// respondTrade reloads a trade after a change and writes it out.
func (h *tradeHandler) respondTrade(c *gin.Context, status int, id uuid.UUID) {
	trade, err := h.loadTrade(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load trade"})
		return
	}
	c.JSON(status, trade)
}

// underlyingOf is the stock a holding's trades are about: an option's
// underlying, or the symbol itself.
func underlyingOf(holding models.Holding) string {
	if holding.Underlying != "" {
		return holding.Underlying
	}
	return holding.Symbol
}

func absQty(txn models.Transaction) float64 {
	if txn.Quantity == nil {
		return 0
	}
	return math.Abs(*txn.Quantity)
}

// allocatedElsewhere is how much of a transaction other trades already hold.
func allocatedElsewhere(tx *gorm.DB, txnID, tradeID uuid.UUID) (float64, error) {
	var sum float64
	err := tx.Model(&models.TradeFill{}).
		Where("transaction_id = ? AND detached = ? AND trade_id <> ?", txnID, false, tradeID).
		Select("COALESCE(SUM(quantity), 0)").Scan(&sum).Error
	return sum, err
}

// snapshotRules copies a strategy's rules onto a trade as its checklist. met
// names the rules the user ticked when starting it: with it, unticked entry and
// risk rules are recorded as not met; exit rules stay unanswered either way,
// since nothing has exited yet.
func snapshotRules(tx *gorm.DB, tradeID, strategyID uuid.UUID, met []uuid.UUID) error {
	var rules []models.StrategyRule
	if err := tx.Where("strategy_id = ?", strategyID).Order("sort_order ASC").Find(&rules).Error; err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	checks := make([]models.TradeRuleCheck, len(rules))
	for i, r := range rules {
		checks[i] = models.TradeRuleCheck{TradeID: tradeID, Kind: r.Kind, Text: r.Text, SortOrder: r.SortOrder}
		if met != nil && r.Kind != models.RuleKindExit {
			v := slices.Contains(met, r.ID)
			checks[i].Met = &v
		}
	}
	return tx.Create(&checks).Error
}

// initialRisk derives a trade's planned risk from its stop and its first
// entry - the plan as it stood when the trade was taken.
func initialRisk(tx *gorm.DB, tradeID uuid.UUID, stop float64) (*float64, error) {
	var fills []models.TradeFill
	if err := tx.Preload("Transaction").Where("trade_id = ? AND detached = ?", tradeID, false).Find(&fills).Error; err != nil {
		return nil, err
	}
	var first *models.TradeFill
	for i := range fills {
		t := fills[i].Transaction
		if t == nil || t.Effect != models.EffectOpen || t.HoldingID == nil {
			continue
		}
		if first == nil || t.Date.Before(first.Transaction.Date) {
			first = &fills[i]
		}
	}
	if first == nil {
		return nil, nil
	}
	var holding models.Holding
	if err := tx.First(&holding, "id = ?", *first.Transaction.HoldingID).Error; err != nil {
		return nil, err
	}
	risk := portfolio.PlannedRisk(*first.Transaction, first.Quantity, stop, holding.Multiplier())
	return &risk, nil
}

// GetTrades godoc
// @Summary      List trades, newest first
// @Tags         trades
// @Produce      json
// @Param        status       query     string  false  "open or closed"
// @Param        strategy_id  query     string  false  "Limit to one strategy"
// @Param        account_id   query     string  false  "Limit to one account"
// @Param        holding_id   query     string  false  "Trades with a fill in this holding"
// @Success      200          {object}  tradeList
// @Failure      400          {object}  map[string]string
// @Failure      500          {object}  map[string]string
// @Router       /trades [get]
func (h *tradeHandler) GetTrades(c *gin.Context) {
	q := withTradeDetail(h.db).Order("opened_on DESC").Order("id DESC")
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	for _, param := range []string{"strategy_id", "account_id"} {
		v := c.Query(param)
		if v == "" {
			continue
		}
		id, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + param})
			return
		}
		q = q.Where(param+" = ?", id)
	}
	if v := c.Query("holding_id"); v != "" {
		holdingID, err := uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid holding_id"})
			return
		}
		inHolding := h.db.Model(&models.TradeFill{}).Select("trade_fills.trade_id").
			Joins("JOIN transactions ON transactions.id = trade_fills.transaction_id").
			Where("transactions.holding_id = ?", holdingID)
		q = q.Where("id IN (?)", inHolding)
	}

	var trades []models.Trade
	if err := q.Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}
	if err := finishTrades(h.db, trades); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to value trades"})
		return
	}
	c.JSON(http.StatusOK, tradeList(trades))
}

// GetTrade godoc
// @Summary      Get a trade with its fills and rule checks
// @Tags         trades
// @Produce      json
// @Param        id   path      string  true  "Trade ID"
// @Success      200  {object}  models.Trade
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /trades/{id} [get]
func (h *tradeHandler) GetTrade(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return
	}
	trade, err := h.loadTrade(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trade not found"})
		return
	}
	c.JSON(http.StatusOK, trade)
}

type createTradeRequest struct {
	// TransactionID is the opening trade the journal entry starts from - a buy,
	// or a sell-to-open for a written option.
	TransactionID uuid.UUID `json:"transaction_id"`
	// Quantity of it that belongs to the trade, in its own shares. Omitted
	// means all of it that no other trade holds.
	Quantity    *float64   `json:"quantity"`
	StrategyID  *uuid.UUID `json:"strategy_id"`
	InitialStop *float64   `json:"initial_stop"`
	Target      *float64   `json:"target"`
	// PlannedRisk overrides the risk derived from the stop.
	PlannedRisk *float64 `json:"planned_risk"`
	Notes       string   `json:"notes"`
	// MetRuleIDs are the strategy's entry and risk rules the setup met.
	// Omitted leaves the checklist unanswered.
	MetRuleIDs []uuid.UUID `json:"met_rule_ids"`
} // @name CreateTradeRequest

// CreateTrade godoc
// @Summary      Start a trade from an opening transaction
// @Description  Creates the trade with the transaction as its first entry, snapshots the strategy's rules onto it, and attaches any later closes that exit it.
// @Tags         trades
// @Accept       json
// @Produce      json
// @Param        trade  body      createTradeRequest  true  "Trade payload"
// @Success      201    {object}  models.Trade
// @Failure      400    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /trades [post]
func (h *tradeHandler) CreateTrade(c *gin.Context) {
	var req createTradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var trade models.Trade
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var txn models.Transaction
		if err := tx.First(&txn, "id = ?", req.TransactionID).Error; err != nil {
			return errTradeInput{"transaction not found"}
		}
		if txn.Effect != models.EffectOpen || txn.HoldingID == nil || txn.Quantity == nil {
			return errTradeInput{"a trade starts from an opening transaction on a holding"}
		}
		var holding models.Holding
		if err := tx.First(&holding, "id = ?", *txn.HoldingID).Error; err != nil {
			return err
		}

		taken, err := allocatedElsewhere(tx, txn.ID, uuid.Nil)
		if err != nil {
			return err
		}
		available := absQty(txn) - taken
		qty := available
		if req.Quantity != nil {
			qty = *req.Quantity
		}
		if qty <= fillEpsilon || qty > available+fillEpsilon {
			return errTradeInput{"quantity must be positive and no more than the transaction's unassigned shares"}
		}

		if req.StrategyID != nil {
			var strategy models.Strategy
			if err := tx.First(&strategy, "id = ?", *req.StrategyID).Error; err != nil {
				return errTradeInput{"strategy not found"}
			}
		}

		direction := txn.Direction
		if direction == "" {
			direction = models.DirectionLong
		}
		opened := txn.Date
		trade = models.Trade{
			AccountID:   txn.AccountID,
			Underlying:  underlyingOf(holding),
			Structure:   models.StructureSingle,
			Direction:   direction,
			Status:      models.TradeStatusOpen,
			OpenedOn:    &opened,
			StrategyID:  req.StrategyID,
			InitialStop: req.InitialStop,
			CurrentStop: req.InitialStop,
			Target:      req.Target,
			PlannedRisk: req.PlannedRisk,
			Notes:       req.Notes,
		}
		if trade.PlannedRisk == nil && req.InitialStop != nil {
			risk := portfolio.PlannedRisk(txn, qty, *req.InitialStop, holding.Multiplier())
			trade.PlannedRisk = &risk
		}
		if err := tx.Create(&trade).Error; err != nil {
			return err
		}
		fill := models.TradeFill{TradeID: trade.ID, TransactionID: txn.ID, Quantity: qty, Source: models.FillSourceManual}
		if err := tx.Create(&fill).Error; err != nil {
			return err
		}
		if req.StrategyID != nil {
			if err := snapshotRules(tx, trade.ID, *req.StrategyID, req.MetRuleIDs); err != nil {
				return err
			}
		}
		return portfolio.SyncHoldingTrades(tx, holding.ID)
	})
	if h.inputError(c, err) {
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create trade"})
		return
	}
	h.respondTrade(c, http.StatusCreated, trade.ID)
}

// inputError writes a 400 for a rejected request and reports whether it did.
func (h *tradeHandler) inputError(c *gin.Context, err error) bool {
	var input errTradeInput
	if errors.As(err, &input) {
		c.JSON(http.StatusBadRequest, gin.H{"error": input.msg})
		return true
	}
	return false
}

// updateTradeRequest is the trade's whole plan: every field is written, so an
// omitted one is cleared.
type updateTradeRequest struct {
	StrategyID  *uuid.UUID `json:"strategy_id"`
	Structure   string     `json:"structure"`
	InitialStop *float64   `json:"initial_stop"`
	CurrentStop *float64   `json:"current_stop"`
	Target      *float64   `json:"target"`
	// PlannedRisk overrides the risk derived from the initial stop; leave it
	// out to derive it.
	PlannedRisk *float64 `json:"planned_risk"`
	Notes       string   `json:"notes"`
	Rating      *int     `json:"rating"`
} // @name UpdateTradeRequest

// UpdateTrade godoc
// @Summary      Update a trade's plan and journal fields
// @Description  Replaces the plan. Moving the trade to a different strategy replaces its rule checklist with the new strategy's, unanswered.
// @Tags         trades
// @Accept       json
// @Produce      json
// @Param        id     path      string              true  "Trade ID"
// @Param        trade  body      updateTradeRequest  true  "Trade payload"
// @Success      200    {object}  models.Trade
// @Failure      400    {object}  map[string]string
// @Failure      404    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /trades/{id} [patch]
func (h *tradeHandler) UpdateTrade(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return
	}
	var req updateTradeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Structure == "" {
		req.Structure = models.StructureSingle
	}
	if !slices.Contains([]string{models.StructureSingle, models.StructureCoveredCall, models.StructureVertical, models.StructureOther}, req.Structure) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown structure"})
		return
	}
	if req.Rating != nil && (*req.Rating < 1 || *req.Rating > 5) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating must be 1 to 5"})
		return
	}

	var trade models.Trade
	if err := h.db.First(&trade, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trade not found"})
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		strategyChanged := (trade.StrategyID == nil) != (req.StrategyID == nil) ||
			(trade.StrategyID != nil && *trade.StrategyID != *req.StrategyID)
		if strategyChanged {
			if req.StrategyID != nil {
				var strategy models.Strategy
				if err := tx.First(&strategy, "id = ?", *req.StrategyID).Error; err != nil {
					return errTradeInput{"strategy not found"}
				}
			}
			if err := tx.Where("trade_id = ?", trade.ID).Delete(&models.TradeRuleCheck{}).Error; err != nil {
				return err
			}
			if req.StrategyID != nil {
				if err := snapshotRules(tx, trade.ID, *req.StrategyID, nil); err != nil {
					return err
				}
			}
		}

		risk := req.PlannedRisk
		if risk == nil && req.InitialStop != nil {
			derived, err := initialRisk(tx, trade.ID, *req.InitialStop)
			if err != nil {
				return err
			}
			risk = derived
		}

		if err := tx.Model(&trade).Select(
			"strategy_id", "structure", "initial_stop", "current_stop", "target",
			"planned_risk", "notes", "rating",
		).Updates(models.Trade{
			StrategyID:  req.StrategyID,
			Structure:   req.Structure,
			InitialStop: req.InitialStop,
			CurrentStop: req.CurrentStop,
			Target:      req.Target,
			PlannedRisk: risk,
			Notes:       req.Notes,
			Rating:      req.Rating,
		}).Error; err != nil {
			return err
		}
		// R depends on the planned risk.
		return portfolio.RecomputeTrade(tx, trade.ID)
	})
	if h.inputError(c, err) {
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update trade"})
		return
	}
	h.respondTrade(c, http.StatusOK, trade.ID)
}

// DeleteTrade godoc
// @Summary      Delete a trade
// @Description  Removes the journal entry, its fills and rule checks. The transactions are untouched; closes it held may be picked up by other open trades.
// @Tags         trades
// @Param        id   path  string  true  "Trade ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /trades/{id} [delete]
func (h *tradeHandler) DeleteTrade(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return
	}
	var trade models.Trade
	if err := h.db.First(&trade, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trade not found"})
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		var holdingIDs []uuid.UUID
		if err := tx.Model(&models.TradeFill{}).
			Joins("JOIN transactions ON transactions.id = trade_fills.transaction_id").
			Where("trade_fills.trade_id = ? AND transactions.holding_id IS NOT NULL", trade.ID).
			Distinct("transactions.holding_id").Pluck("transactions.holding_id", &holdingIDs).Error; err != nil {
			return err
		}
		if err := tx.Where("trade_id = ?", trade.ID).Delete(&models.TradeFill{}).Error; err != nil {
			return err
		}
		if err := tx.Where("trade_id = ?", trade.ID).Delete(&models.TradeRuleCheck{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&trade).Error; err != nil {
			return err
		}
		// Closes this trade held are free again; another trade may want them.
		for _, hid := range holdingIDs {
			if err := portfolio.SyncHoldingTrades(tx, hid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete trade"})
		return
	}
	c.Status(http.StatusNoContent)
}

type addFillRequest struct {
	TransactionID uuid.UUID `json:"transaction_id"`
	// Quantity in the transaction's own shares; omitted means all of it that
	// no other trade holds.
	Quantity *float64 `json:"quantity"`
} // @name AddFillRequest

// AddFill godoc
// @Summary      Attach a transaction to a trade
// @Description  Adds an entry (a scale-in, or another leg on the same underlying) or an exit. An exit given without a quantity is sized like an auto fill, to what the trade had open; with one it is pinned as a manual fill.
// @Tags         trades
// @Accept       json
// @Produce      json
// @Param        id    path      string          true  "Trade ID"
// @Param        fill  body      addFillRequest  true  "Fill payload"
// @Success      200   {object}  models.Trade
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /trades/{id}/fills [post]
func (h *tradeHandler) AddFill(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return
	}
	var req addFillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var trade models.Trade
	if err := h.db.First(&trade, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trade not found"})
		return
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		var txn models.Transaction
		if err := tx.First(&txn, "id = ?", req.TransactionID).Error; err != nil {
			return errTradeInput{"transaction not found"}
		}
		if txn.HoldingID == nil || txn.Quantity == nil || (txn.Effect != models.EffectOpen && txn.Effect != models.EffectClose) {
			return errTradeInput{"only opening and closing transactions on a holding can be fills"}
		}
		if txn.AccountID != trade.AccountID {
			return errTradeInput{"the transaction is in a different account"}
		}
		var holding models.Holding
		if err := tx.First(&holding, "id = ?", *txn.HoldingID).Error; err != nil {
			return err
		}
		if underlyingOf(holding) != trade.Underlying {
			return errTradeInput{"the transaction is on a different underlying"}
		}

		var fill models.TradeFill
		err = tx.Where("trade_id = ? AND transaction_id = ?", trade.ID, txn.ID).First(&fill).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			fill = models.TradeFill{TradeID: trade.ID, TransactionID: txn.ID}
		case err != nil:
			return err
		}
		fill.Detached = false

		// A close with no quantity is left to the sync to size, as if it had
		// been auto-attached: it takes only what the trade had open at the
		// time, rather than the whole sale.
		sized := txn.Effect == models.EffectClose && req.Quantity == nil
		if sized {
			fill.Source = models.FillSourceAuto
		} else {
			taken, err := allocatedElsewhere(tx, txn.ID, trade.ID)
			if err != nil {
				return err
			}
			available := absQty(txn) - taken
			qty := available
			if req.Quantity != nil {
				qty = *req.Quantity
			}
			if qty <= fillEpsilon || qty > available+fillEpsilon {
				return errTradeInput{"quantity must be positive and no more than the transaction's unassigned shares"}
			}
			fill.Quantity = qty
			fill.Source = models.FillSourceManual
		}
		if err := tx.Save(&fill).Error; err != nil {
			return err
		}
		if err := portfolio.SyncHoldingTrades(tx, holding.ID); err != nil {
			return err
		}
		if sized {
			var count int64
			if err := tx.Model(&models.TradeFill{}).Where("id = ?", fill.ID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return errTradeInput{"the trade had nothing open for this to close; give a quantity to attach it anyway"}
			}
		}
		return portfolio.RecomputeTrade(tx, trade.ID)
	})
	if h.inputError(c, err) {
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add fill"})
		return
	}
	h.respondTrade(c, http.StatusOK, trade.ID)
}

type updateFillRequest struct {
	ExitReason string `json:"exit_reason"`
	// Quantity, when set, pins the fill at that size; an auto fill becomes a
	// manual one, so the sync stops resizing it.
	Quantity *float64 `json:"quantity"`
} // @name UpdateFillRequest

// UpdateFill godoc
// @Summary      Record an exit reason or resize a fill
// @Tags         trades
// @Accept       json
// @Produce      json
// @Param        id       path      string             true  "Trade ID"
// @Param        fill_id  path      string             true  "Fill ID"
// @Param        fill     body      updateFillRequest  true  "Fill payload"
// @Success      200      {object}  models.Trade
// @Failure      400      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /trades/{id}/fills/{fill_id} [patch]
func (h *tradeHandler) UpdateFill(c *gin.Context) {
	fill, ok := h.findFill(c)
	if !ok {
		return
	}
	var req updateFillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ExitReason != "" && !slices.Contains(models.ExitReasons, req.ExitReason) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown exit reason"})
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var txn models.Transaction
		if err := tx.First(&txn, "id = ?", fill.TransactionID).Error; err != nil {
			return err
		}
		if req.ExitReason != "" && txn.Effect != models.EffectClose {
			return errTradeInput{"only an exit has an exit reason"}
		}
		updates := map[string]any{"exit_reason": req.ExitReason}
		if req.Quantity != nil {
			taken, err := allocatedElsewhere(tx, txn.ID, fill.TradeID)
			if err != nil {
				return err
			}
			if *req.Quantity <= fillEpsilon || *req.Quantity > absQty(txn)-taken+fillEpsilon {
				return errTradeInput{"quantity must be positive and no more than the transaction's unassigned shares"}
			}
			updates["quantity"] = *req.Quantity
			updates["source"] = models.FillSourceManual
		}
		if err := tx.Model(&models.TradeFill{}).Where("id = ?", fill.ID).Updates(updates).Error; err != nil {
			return err
		}
		if req.Quantity == nil || txn.HoldingID == nil {
			return nil
		}
		if err := portfolio.SyncHoldingTrades(tx, *txn.HoldingID); err != nil {
			return err
		}
		return portfolio.RecomputeTrade(tx, fill.TradeID)
	})
	if h.inputError(c, err) {
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update fill"})
		return
	}
	h.respondTrade(c, http.StatusOK, fill.TradeID)
}

// RemoveFill godoc
// @Summary      Take a fill off a trade
// @Description  A manual fill is removed. An auto fill is detached instead, so the sync doesn't attach it again; adding it back re-attaches it.
// @Tags         trades
// @Produce      json
// @Param        id       path      string  true  "Trade ID"
// @Param        fill_id  path      string  true  "Fill ID"
// @Success      200      {object}  models.Trade
// @Failure      400      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /trades/{id}/fills/{fill_id} [delete]
func (h *tradeHandler) RemoveFill(c *gin.Context) {
	fill, ok := h.findFill(c)
	if !ok {
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if fill.Source == models.FillSourceAuto {
			if err := tx.Model(&models.TradeFill{}).Where("id = ?", fill.ID).Update("detached", true).Error; err != nil {
				return err
			}
		} else if err := tx.Delete(&models.TradeFill{}, "id = ?", fill.ID).Error; err != nil {
			return err
		}
		var txn models.Transaction
		if err := tx.Unscoped().First(&txn, "id = ?", fill.TransactionID).Error; err != nil {
			return err
		}
		if txn.HoldingID != nil {
			if err := portfolio.SyncHoldingTrades(tx, *txn.HoldingID); err != nil {
				return err
			}
		}
		return portfolio.RecomputeTrade(tx, fill.TradeID)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove fill"})
		return
	}
	h.respondTrade(c, http.StatusOK, fill.TradeID)
}

// findFill resolves the trade and fill ids in the path, writing the error
// response itself when either is wrong.
func (h *tradeHandler) findFill(c *gin.Context) (models.TradeFill, bool) {
	var fill models.TradeFill
	tradeID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return fill, false
	}
	fillID, err := uuid.Parse(c.Param("fill_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid fill id"})
		return fill, false
	}
	if err := h.db.First(&fill, "id = ? AND trade_id = ?", fillID, tradeID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "fill not found"})
		return fill, false
	}
	return fill, true
}

type ruleCheckInput struct {
	ID  uuid.UUID `json:"id"`
	Met *bool     `json:"met"`
} // @name RuleCheckInput

// UpdateRuleChecks godoc
// @Summary      Answer a trade's rule checklist
// @Description  Sets met (true, false, or null for unanswered) on each check listed; checks not listed are left alone.
// @Tags         trades
// @Accept       json
// @Produce      json
// @Param        id      path      string            true  "Trade ID"
// @Param        checks  body      []ruleCheckInput  true  "Checks"
// @Success      200     {object}  models.Trade
// @Failure      400     {object}  map[string]string
// @Failure      500     {object}  map[string]string
// @Router       /trades/{id}/rule-checks [put]
func (h *tradeHandler) UpdateRuleChecks(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return
	}
	var req []ruleCheckInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, in := range req {
			if err := tx.Model(&models.TradeRuleCheck{}).
				Where("id = ? AND trade_id = ?", in.ID, id).
				Update("met", in.Met).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update rule checks"})
		return
	}
	h.respondTrade(c, http.StatusOK, id)
}

// tradeCandidate is a transaction that could be added to a trade, and how much
// of it is free to add.
type tradeCandidate struct {
	Transaction       models.Transaction `json:"transaction"`
	AvailableQuantity float64            `json:"available_quantity"`
	// Detached is set when it was auto-attached to this trade once and the
	// user took it off.
	Detached bool `json:"detached"`
} // @name TradeCandidate

type tradeCandidateList []tradeCandidate // @name TradeCandidates

// GetCandidates godoc
// @Summary      Transactions that could be added to a trade
// @Description  Opening and closing transactions in the trade's account, on its underlying (the stock and its options), since it opened, with shares no trade holds yet.
// @Tags         trades
// @Produce      json
// @Param        id   path      string  true  "Trade ID"
// @Success      200  {object}  tradeCandidateList
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /trades/{id}/candidates [get]
func (h *tradeHandler) GetCandidates(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid trade id"})
		return
	}
	var trade models.Trade
	if err := h.db.Preload("Fills").First(&trade, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trade not found"})
		return
	}
	attached := map[uuid.UUID]bool{}
	detached := map[uuid.UUID]bool{}
	for _, f := range trade.Fills {
		if f.Detached {
			detached[f.TransactionID] = true
		} else {
			attached[f.TransactionID] = true
		}
	}

	var holdingIDs []uuid.UUID
	if err := h.db.Model(&models.Holding{}).
		Where("symbol = ? OR underlying = ?", trade.Underlying, trade.Underlying).
		Pluck("id", &holdingIDs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch candidates"})
		return
	}
	q := h.db.Where("holding_id IN ? AND account_id = ? AND effect IN ? AND quantity IS NOT NULL",
		holdingIDs, trade.AccountID, []string{models.EffectOpen, models.EffectClose})
	if trade.OpenedOn != nil {
		q = q.Where("date >= ?", *trade.OpenedOn)
	}
	var txns []models.Transaction
	if err := q.Order("date ASC").Order("id ASC").Find(&txns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch candidates"})
		return
	}

	candidates := tradeCandidateList{}
	for _, txn := range txns {
		if attached[txn.ID] {
			continue
		}
		taken, err := allocatedElsewhere(h.db, txn.ID, trade.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch candidates"})
			return
		}
		available := absQty(txn) - taken
		if available <= fillEpsilon {
			continue
		}
		candidates = append(candidates, tradeCandidate{Transaction: txn, AvailableQuantity: available, Detached: detached[txn.ID]})
	}
	c.JSON(http.StatusOK, candidates)
}
