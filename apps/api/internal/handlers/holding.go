package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/marketdata"
	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HoldingHandler interface {
	GetHoldings(c *gin.Context)
	CreateHolding(c *gin.Context)
	UpdateHolding(c *gin.Context)
	BackfillProfiles(c *gin.Context)
	GetAllocation(c *gin.Context)
}

type holdingHandler struct {
	db       *gorm.DB
	enricher *marketdata.Enricher
}

func NewHoldingHandler(db *gorm.DB, enricher *marketdata.Enricher) HoldingHandler {
	return &holdingHandler{db: db, enricher: enricher}
}

type paginatedHoldings struct {
	Data     []models.Holding `json:"data"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
} // @name PaginatedHoldings

type createHoldingRequest struct {
	AssetType        string  `json:"asset_type" binding:"required"`
	Symbol           string  `json:"symbol"`
	Description      string  `json:"description" binding:"required"`
	Status           string  `json:"status"`
	LastPrice        float64 `json:"last_price"`
	Quantity         float64 `json:"purchase_quantity"`
	CurrentValue     float64 `json:"current_value"`
	AverageCostBasis float64 `json:"average_cost_basis"`
	CostBasisTotal   float64 `json:"cost_basis_total"`
	DividendIncome   float64 `json:"dividend_income"`

	// TaxClassOverride and IssuerJurisdiction describe the asset's tax
	// attributes; nil means "derive from the asset type". They replace the old
	// state_tax_exempt flag - what a holding pays and who issued it are facts
	// about the asset, while whether that's exempt is a jurisdiction's rule.
	TaxClassOverride   *string `json:"tax_class_override"`
	IssuerJurisdiction *string `json:"issuer_jurisdiction"`
} // @name CreateHoldingRequest

// CreateHolding godoc
// @Summary      Create a new holding
// @Tags         holdings
// @Accept       json
// @Produce      json
// @Param        holding  body      createHoldingRequest  true  "Holding payload"
// @Success      201      {object}  models.Holding
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /holdings [post]
func (h *holdingHandler) CreateHolding(c *gin.Context) {
	var req createHoldingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := req.Status
	if status == "" {
		status = models.HoldingStatusOpen
	}

	holding := models.Holding{
		AssetType:        req.AssetType,
		Symbol:           req.Symbol,
		Description:      req.Description,
		Status:           status,
		LastPrice:        req.LastPrice,
		Quantity:         req.Quantity,
		CurrentValue:     req.CurrentValue,
		AverageCostBasis: req.AverageCostBasis,
		CostBasisTotal:   req.CostBasisTotal,
		DividendIncome:   req.DividendIncome,

		TaxClassOverride:   req.TaxClassOverride,
		IssuerJurisdiction: req.IssuerJurisdiction,
	}

	if err := h.db.Create(&holding).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create holding"})
		return
	}

	// Reference data - sector, industry, the company's name - is a bonus on top
	// of a holding that already exists, so a provider that's slow or down can
	// only cost us the extra detail, never the holding.
	h.enricher.EnrichQuietly(c.Request.Context(), h.db, &holding)

	holding.TaxClass = holding.ResolveTaxClass()
	holding.AssetClass = holding.ResolveAssetClass()
	c.JSON(http.StatusCreated, holding)
}

type updateHoldingRequest struct {
	AssetType        string  `json:"asset_type"`
	Symbol           string  `json:"symbol"`
	Description      string  `json:"description"`
	Status           string  `json:"status"`
	LastPrice        float64 `json:"last_price"`
	Quantity         float64 `json:"purchase_quantity"`
	CurrentValue     float64 `json:"current_value"`
	AverageCostBasis float64 `json:"average_cost_basis"`
	CostBasisTotal   float64 `json:"cost_basis_total"`
	DividendIncome   float64 `json:"dividend_income"`

	TaxClassOverride   *string `json:"tax_class_override"`
	IssuerJurisdiction *string `json:"issuer_jurisdiction"`
} // @name UpdateHoldingRequest

// UpdateHolding godoc
// @Summary      Update an existing holding
// @Tags         holdings
// @Accept       json
// @Produce      json
// @Param        id       path      string                true  "Holding ID"
// @Param        holding  body      updateHoldingRequest  true  "Holding payload"
// @Success      200      {object}  models.Holding
// @Failure      400      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /holdings/{id} [patch]
func (h *holdingHandler) UpdateHolding(c *gin.Context) {
	id := c.Param("id")

	var holding models.Holding
	if err := h.db.First(&holding, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "holding not found"})
		return
	}

	var req updateHoldingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Remember the tax attributes as they stand: if the edit changes what this
	// asset pays or who issued it, every gain and distribution already realized
	// from it needs re-evaluating against the rules.
	priorTaxClass := holding.ResolveTaxClass()
	priorIssuer := holding.ResolveIssuerJurisdiction()

	if req.AssetType != "" {
		holding.AssetType = req.AssetType
	}
	if req.Symbol != "" {
		holding.Symbol = req.Symbol
	}
	if req.Description != "" {
		holding.Description = req.Description
	}
	if req.Status != "" {
		holding.Status = req.Status
	}
	holding.LastPrice = req.LastPrice
	holding.Quantity = req.Quantity
	holding.CurrentValue = req.CurrentValue
	holding.AverageCostBasis = req.AverageCostBasis
	holding.CostBasisTotal = req.CostBasisTotal
	holding.DividendIncome = req.DividendIncome
	// Authoritative: nil clears the override back to the asset-type default.
	holding.TaxClassOverride = req.TaxClassOverride
	holding.IssuerJurisdiction = req.IssuerJurisdiction

	holding.GainUnrealizedAmount = holding.CurrentValue - holding.CostBasisTotal
	// Avoid NaN/Inf (which JSON can't marshal) when there's no cost basis yet.
	if holding.CostBasisTotal != 0 {
		holding.GainUnrealizedPercent = holding.GainUnrealizedAmount / holding.CostBasisTotal * 100
	} else {
		holding.GainUnrealizedPercent = 0
	}

	taxChanged := holding.ResolveTaxClass() != priorTaxClass || holding.ResolveIssuerJurisdiction() != priorIssuer

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&holding).Error; err != nil {
			return err
		}
		if !taxChanged {
			return nil
		}
		return portfolio.RetaxHolding(tx, holding.ID)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update holding"})
		return
	}

	holding.TaxClass = holding.ResolveTaxClass()
	holding.AssetClass = holding.ResolveAssetClass()
	c.JSON(http.StatusOK, holding)
}

// parseInputDate accepts either a plain date (YYYY-MM-DD, as an HTML date
// input produces) or a full RFC3339 timestamp.
func parseInputDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// GetHoldings godoc
// @Summary      List holdings with pagination
// @Tags         holdings
// @Produce      json
// @Param        page       query     int  false  "Page number (default 1)"
// @Param        page_size  query     int  false  "Items per page (default 20, max 100)"
// @Success      200        {object}  paginatedHoldings
// @Failure      500        {object}  map[string]string
// @Router       /holdings [get]
func (h *holdingHandler) GetHoldings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	if err := h.db.Model(&models.Holding{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch holdings"})
		return
	}

	var holdings []models.Holding
	offset := (page - 1) * pageSize
	if err := h.db.Offset(offset).Limit(pageSize).Find(&holdings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch holdings"})
		return
	}

	c.JSON(http.StatusOK, paginatedHoldings{
		Data:     holdings,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// BackfillProfiles godoc
// @Summary      Look up reference data for holdings that don't have it yet
// @Tags         holdings
// @Produce      json
// @Success      200  {object}  marketdata.BackfillResult
// @Failure      503  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /holdings/backfill-profiles [post]
//
// Holdings created before a market-data key was configured, or while the
// provider was unreachable, have no sector or industry. This fills them in.
// Requests are paced, so a large portfolio takes a while.
func (h *holdingHandler) BackfillProfiles(c *gin.Context) {
	if !h.enricher.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "market data is not configured; set FMP_API_KEY to enable profile lookups",
		})
		return
	}

	result, err := h.enricher.Backfill(c.Request.Context(), h.db, 250*time.Millisecond)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to backfill profiles"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// allocationSlice is one wedge of a breakdown.
type allocationSlice struct {
	Label   string  `json:"label"`
	Value   float64 `json:"value"`
	Percent float64 `json:"percent"`
} // @name AllocationSlice

type allocationSummary struct {
	TotalValue float64 `json:"total_value"`

	// AssetClasses splits the whole portfolio; EquitySectors splits the equity
	// part of it and is weighted within that, since a sector describes a company
	// and means nothing for cash or a government bond.
	AssetClasses  []allocationSlice `json:"asset_classes"`
	EquityValue   float64           `json:"equity_value"`
	EquityPercent float64           `json:"equity_percent"`
	EquitySectors []allocationSlice `json:"equity_sectors"`
} // @name AllocationSummary

// GetAllocation godoc
// @Summary      Portfolio allocation by asset class, and sector within equities
// @Tags         holdings
// @Produce      json
// @Success      200  {object}  allocationSummary
// @Failure      500  {object}  map[string]string
// @Router       /holdings/allocation [get]
//
// Computed over every holding rather than a page of them. An allocation drawn
// from whatever rows the table happens to be showing describes the page, not
// the portfolio - which is wrong by exactly as much as the paging hides.
func (h *holdingHandler) GetAllocation(c *gin.Context) {
	var holdings []models.Holding
	if err := h.db.Find(&holdings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch holdings"})
		return
	}

	byClass := map[string]float64{}
	bySector := map[string]float64{}
	var total, equity float64

	for _, holding := range holdings {
		value := holding.CurrentValue
		if value <= 0 {
			continue
		}
		total += value

		class := holding.ResolveAssetClass()
		byClass[class] += value

		if class != models.AssetClassEquity {
			continue
		}
		equity += value
		// A holding whose profile lookup hasn't run yet is named rather than
		// dropped, so the sector weights still add to the equity total.
		sector := holding.Sector
		if sector == "" {
			sector = "Unclassified"
		}
		bySector[sector] += value
	}

	summary := allocationSummary{
		TotalValue:    total,
		EquityValue:   equity,
		AssetClasses:  slicesOf(byClass, total),
		EquitySectors: slicesOf(bySector, equity),
	}
	if total > 0 {
		summary.EquityPercent = equity / total * 100
	}
	c.JSON(http.StatusOK, summary)
}

// slicesOf turns a label/value map into percentage slices, largest first.
func slicesOf(values map[string]float64, total float64) []allocationSlice {
	out := make([]allocationSlice, 0, len(values))
	for label, value := range values {
		slice := allocationSlice{Label: label, Value: value}
		if total > 0 {
			slice.Percent = value / total * 100
		}
		out = append(out, slice)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out
}
