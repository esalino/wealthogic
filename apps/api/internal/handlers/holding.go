package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HoldingHandler interface {
	GetHoldings(c *gin.Context)
	CreateHolding(c *gin.Context)
	UpdateHolding(c *gin.Context)
}

type holdingHandler struct {
	db *gorm.DB
}

func NewHoldingHandler(db *gorm.DB) HoldingHandler {
	return &holdingHandler{db: db}
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
	}

	if err := h.db.Create(&holding).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create holding"})
		return
	}

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

	holding.GainUnrealizedAmount = holding.CurrentValue - holding.CostBasisTotal
	holding.GainUnrealizedPercent = holding.GainUnrealizedAmount / holding.CostBasisTotal * 100

	if err := h.db.Save(&holding).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update holding"})
		return
	}

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
