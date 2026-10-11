package handlers

import (
	"net/http"
	"slices"
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type StrategyHandler interface {
	GetStrategies(c *gin.Context)
	CreateStrategy(c *gin.Context)
	UpdateStrategy(c *gin.Context)
	DeleteStrategy(c *gin.Context)
	GetPerformance(c *gin.Context)
}

type strategyHandler struct {
	db *gorm.DB
}

func NewStrategyHandler(db *gorm.DB) StrategyHandler {
	return &strategyHandler{db: db}
}

type strategyList []models.Strategy // @name Strategies

// preloadRules loads a strategy's rules in checklist order.
func preloadRules(db *gorm.DB) *gorm.DB {
	return db.Order("sort_order ASC").Order("id ASC")
}

// GetStrategies godoc
// @Summary      List strategies with their rules
// @Tags         strategies
// @Produce      json
// @Param        status  query     string  false  "active or retired; omit for both"
// @Success      200     {object}  strategyList
// @Failure      500     {object}  map[string]string
// @Router       /strategies [get]
func (h *strategyHandler) GetStrategies(c *gin.Context) {
	q := h.db.Preload("Rules", preloadRules).Order("name ASC")
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	var strategies []models.Strategy
	if err := q.Find(&strategies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch strategies"})
		return
	}
	c.JSON(http.StatusOK, strategyList(strategies))
}

type strategyRuleInput struct {
	Kind string `json:"kind" binding:"required"`
	Text string `json:"text" binding:"required"`
} // @name StrategyRuleInput

// strategyRequest is the whole strategy. Rules are listed in checklist order
// and replace the strategy's existing rules; trades already started keep the
// rules they were judged by.
type strategyRequest struct {
	Name        string              `json:"name" binding:"required"`
	Description string              `json:"description"`
	Status      string              `json:"status"`
	Direction   string              `json:"direction"`
	Timeframe   string              `json:"timeframe"`
	Rules       []strategyRuleInput `json:"rules"`
} // @name StrategyRequest

// validate normalizes the request and reports what's wrong with it, if
// anything.
func (r *strategyRequest) validate() string {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return "name is required"
	}
	if r.Status == "" {
		r.Status = models.StrategyStatusActive
	}
	if r.Status != models.StrategyStatusActive && r.Status != models.StrategyStatusRetired {
		return "status must be active or retired"
	}
	if !slices.Contains([]string{"", models.DirectionLong, models.DirectionShort, "both"}, r.Direction) {
		return "direction must be long, short or both"
	}
	for i := range r.Rules {
		r.Rules[i].Text = strings.TrimSpace(r.Rules[i].Text)
		if !slices.Contains([]string{models.RuleKindEntry, models.RuleKindExit, models.RuleKindRisk}, r.Rules[i].Kind) {
			return "rule kind must be entry, exit or risk"
		}
		if r.Rules[i].Text == "" {
			return "rule text is required"
		}
	}
	return ""
}

func (r strategyRequest) rules(strategyID uuid.UUID) []models.StrategyRule {
	rules := make([]models.StrategyRule, len(r.Rules))
	for i, in := range r.Rules {
		rules[i] = models.StrategyRule{StrategyID: strategyID, Kind: in.Kind, Text: in.Text, SortOrder: i}
	}
	return rules
}

// CreateStrategy godoc
// @Summary      Create a strategy
// @Tags         strategies
// @Accept       json
// @Produce      json
// @Param        strategy  body      strategyRequest  true  "Strategy payload"
// @Success      201       {object}  models.Strategy
// @Failure      400       {object}  map[string]string
// @Failure      500       {object}  map[string]string
// @Router       /strategies [post]
func (h *strategyHandler) CreateStrategy(c *gin.Context) {
	var req strategyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if msg := req.validate(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	strategy := models.Strategy{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
		Direction:   req.Direction,
		Timeframe:   req.Timeframe,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Rules").Create(&strategy).Error; err != nil {
			return err
		}
		strategy.Rules = req.rules(strategy.ID)
		if len(strategy.Rules) == 0 {
			return nil
		}
		return tx.Create(&strategy.Rules).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create strategy"})
		return
	}
	c.JSON(http.StatusCreated, strategy)
}

// UpdateStrategy godoc
// @Summary      Replace a strategy and its rules
// @Description  Trades already started under the strategy keep the rules they were judged by.
// @Tags         strategies
// @Accept       json
// @Produce      json
// @Param        id        path      string           true  "Strategy ID"
// @Param        strategy  body      strategyRequest  true  "Strategy payload"
// @Success      200       {object}  models.Strategy
// @Failure      400       {object}  map[string]string
// @Failure      404       {object}  map[string]string
// @Failure      500       {object}  map[string]string
// @Router       /strategies/{id} [put]
func (h *strategyHandler) UpdateStrategy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid strategy id"})
		return
	}
	var req strategyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if msg := req.validate(); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}

	var strategy models.Strategy
	if err := h.db.First(&strategy, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "strategy not found"})
		return
	}
	strategy.Name = req.Name
	strategy.Description = req.Description
	strategy.Status = req.Status
	strategy.Direction = req.Direction
	strategy.Timeframe = req.Timeframe

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Rules").Save(&strategy).Error; err != nil {
			return err
		}
		if err := tx.Where("strategy_id = ?", strategy.ID).Delete(&models.StrategyRule{}).Error; err != nil {
			return err
		}
		strategy.Rules = req.rules(strategy.ID)
		if len(strategy.Rules) == 0 {
			return nil
		}
		return tx.Create(&strategy.Rules).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update strategy"})
		return
	}
	c.JSON(http.StatusOK, strategy)
}

// DeleteStrategy godoc
// @Summary      Delete a strategy
// @Description  Soft-deletes it. Trades taken under it keep the link and their rule checks.
// @Tags         strategies
// @Param        id   path  string  true  "Strategy ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /strategies/{id} [delete]
func (h *strategyHandler) DeleteStrategy(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid strategy id"})
		return
	}
	res := h.db.Delete(&models.Strategy{}, "id = ?", id)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete strategy"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "strategy not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// strategyPerformance is the performance response: every strategy with closed
// trades (plus one row, with a nil strategy_id, for trades taken under none),
// and a total across all of them.
type strategyPerformance struct {
	Strategies []portfolio.TradeStats `json:"strategies"`
	Total      portfolio.TradeStats   `json:"total"`
} // @name StrategyPerformance

// GetPerformance godoc
// @Summary      Closed-trade performance by strategy
// @Tags         strategies
// @Produce      json
// @Param        account_id  query     string  false  "Limit to one account"
// @Success      200         {object}  strategyPerformance
// @Failure      400         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /strategies/performance [get]
func (h *strategyHandler) GetPerformance(c *gin.Context) {
	q := h.db.Preload("RuleChecks").Where("status = ?", models.TradeStatusClosed)
	if aid := c.Query("account_id"); aid != "" {
		accountID, err := uuid.Parse(aid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account_id"})
			return
		}
		q = q.Where("account_id = ?", accountID)
	}
	var trades []models.Trade
	if err := q.Find(&trades).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch trades"})
		return
	}

	// Deleted strategies still name the trades taken under them.
	var strategies []models.Strategy
	if err := h.db.Unscoped().Order("name ASC").Find(&strategies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch strategies"})
		return
	}

	groups := map[uuid.UUID][]models.Trade{}
	var unassigned []models.Trade
	for _, t := range trades {
		if t.StrategyID == nil {
			unassigned = append(unassigned, t)
			continue
		}
		groups[*t.StrategyID] = append(groups[*t.StrategyID], t)
	}

	resp := strategyPerformance{Strategies: []portfolio.TradeStats{}, Total: portfolio.SummarizeTrades(trades)}
	for _, s := range strategies {
		group, ok := groups[s.ID]
		if !ok {
			continue
		}
		stats := portfolio.SummarizeTrades(group)
		id := s.ID
		stats.StrategyID = &id
		stats.StrategyName = s.Name
		resp.Strategies = append(resp.Strategies, stats)
	}
	if len(unassigned) > 0 {
		stats := portfolio.SummarizeTrades(unassigned)
		stats.StrategyName = "No strategy"
		resp.Strategies = append(resp.Strategies, stats)
	}
	c.JSON(http.StatusOK, resp)
}
