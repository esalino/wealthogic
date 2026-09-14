package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/eriksalino/wealthogic/api/internal/tax"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DistributionHandler interface {
	GetDistributions(c *gin.Context)
	CreateDistribution(c *gin.Context)
	UpdateDistribution(c *gin.Context)
	DeleteDistribution(c *gin.Context)
}

type distributionHandler struct {
	db *gorm.DB
}

func NewDistributionHandler(db *gorm.DB) DistributionHandler {
	return &distributionHandler{db: db}
}

// distributionSummary totals a year's income, independent of pagination.
//
// It deliberately carries no tax-bucket split: how income divides into
// qualified, ordinary, and exempt depends on the jurisdiction asking, so that
// breakdown belongs to GET /tax/summary, which reports it per jurisdiction from
// the stored treatments. Total is the one figure that means the same everywhere.
type distributionSummary struct {
	Total float64 `json:"total"`
} // @name DistributionSummary

type paginatedDistributions struct {
	Data     []models.Distribution `json:"data"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Summary  distributionSummary   `json:"summary"`
} // @name PaginatedDistributions

// GetDistributions godoc
// @Summary      List income distributions (dividends/interest) with pagination and a year summary
// @Tags         distributions
// @Produce      json
// @Param        year        query     int     false  "Filter to the tax year the income was paid"
// @Param        holding_id  query     string  false  "Filter to a single holding"
// @Param        page        query     int     false  "Page number (default 1)"
// @Param        page_size   query     int     false  "Items per page (default 20, max 100)"
// @Success      200         {object}  paginatedDistributions
// @Failure      400         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /distributions [get]
func (h *distributionHandler) GetDistributions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Optional holding filter. A security's dividends can predate its first buy
	// (dividend rows never create a holding), so those carry the symbol but a
	// null holding_id - match on either so the holding's tab is complete.
	holdingFilter := func(q *gorm.DB) *gorm.DB { return q }
	if hid := c.Query("holding_id"); hid != "" {
		holdingID, err := uuid.Parse(hid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid holding_id"})
			return
		}
		var holding models.Holding
		if err := h.db.First(&holding, "id = ?", holdingID).Error; err == nil && holding.Symbol != "" {
			sym := holding.Symbol
			holdingFilter = func(q *gorm.DB) *gorm.DB { return q.Where("holding_id = ? OR symbol = ?", holdingID, sym) }
		} else {
			holdingFilter = func(q *gorm.DB) *gorm.DB { return q.Where("holding_id = ?", holdingID) }
		}
	}

	// Combined filter (holding + optional tax year on the payment date), applied
	// to the count, the summary, and the page query alike.
	filter := func(q *gorm.DB) *gorm.DB {
		q = holdingFilter(q)
		if y := c.Query("year"); y != "" {
			if year, err := strconv.Atoi(y); err == nil {
				q = q.Where("EXTRACT(YEAR FROM payment_date) = ?", year)
			}
		}
		return q
	}

	var total int64
	if err := filter(h.db.Model(&models.Distribution{})).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch distributions"})
		return
	}

	// Year total across all matching rows, not just the page.
	var summary distributionSummary
	if err := filter(h.db.Model(&models.Distribution{})).
		Select("COALESCE(SUM(amount), 0) AS total").
		Scan(&summary).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize distributions"})
		return
	}

	var distributions []models.Distribution
	offset := (page - 1) * pageSize
	if err := filter(h.db).Order("payment_date DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&distributions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch distributions"})
		return
	}

	c.JSON(http.StatusOK, paginatedDistributions{
		Data:     distributions,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Summary:  summary,
	})
}

// distributionRequest is the payload for creating or updating an income
// payment by hand. Uploads build the same record; this is the other way in.
type distributionRequest struct {
	AccountID   uuid.UUID  `json:"account_id" binding:"required"`
	HoldingID   *uuid.UUID `json:"holding_id"`
	Category    string     `json:"category"`
	Symbol      string     `json:"symbol"`
	AssetType   *string    `json:"asset_type"`
	PaymentDate string     `json:"payment_date" binding:"required"`
	Amount      float64    `json:"amount"`
} // @name DistributionRequest

// bind validates the payload and applies it to a distribution.
func (r distributionRequest) bind(d *models.Distribution) error {
	paymentDate, err := parseInputDate(r.PaymentDate)
	if err != nil {
		return err
	}

	category := r.Category
	if category == "" {
		category = models.IncomeTypeDividend
	}
	if category != models.IncomeTypeDividend && category != models.IncomeTypeInterest {
		return errUnknownCategory
	}

	d.AccountID = r.AccountID
	d.HoldingID = r.HoldingID
	d.Category = category
	d.Symbol = strings.TrimSpace(r.Symbol)
	d.AssetType = r.AssetType
	d.PaymentDate = paymentDate
	d.Amount = r.Amount
	return nil
}

var errUnknownCategory = errors.New("category must be dividend or interest")

// save persists a distribution and rebuilds the realized event derived from it,
// in one transaction. The ledger below a source record is always derived, so
// every write goes through the same replay rather than patching it in place.
func (h *distributionHandler) save(d *models.Distribution) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(d).Error; err != nil {
			return err
		}
		applier, err := tax.NewApplier(tx)
		if err != nil {
			return err
		}
		return portfolio.RealizeDistribution(tx, d, applier)
	})
}

// CreateDistribution godoc
// @Summary      Record an income payment by hand
// @Tags         distributions
// @Accept       json
// @Produce      json
// @Param        distribution  body      distributionRequest  true  "Distribution payload"
// @Success      201           {object}  models.Distribution
// @Failure      400           {object}  map[string]string
// @Failure      500           {object}  map[string]string
// @Router       /distributions [post]
func (h *distributionHandler) CreateDistribution(c *gin.Context) {
	var req distributionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var dist models.Distribution
	if err := req.bind(&dist); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.save(&dist); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create distribution"})
		return
	}
	c.JSON(http.StatusCreated, dist)
}

// UpdateDistribution godoc
// @Summary      Update an income payment
// @Tags         distributions
// @Accept       json
// @Produce      json
// @Param        id            path      string               true  "Distribution ID"
// @Param        distribution  body      distributionRequest  true  "Distribution payload"
// @Success      200           {object}  models.Distribution
// @Failure      400           {object}  map[string]string
// @Failure      404           {object}  map[string]string
// @Failure      500           {object}  map[string]string
// @Router       /distributions/{id} [patch]
func (h *distributionHandler) UpdateDistribution(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid distribution id"})
		return
	}

	var dist models.Distribution
	if err := h.db.First(&dist, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "distribution not found"})
		return
	}

	var req distributionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.bind(&dist); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.save(&dist); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update distribution"})
		return
	}
	c.JSON(http.StatusOK, dist)
}

// DeleteDistribution godoc
// @Summary      Delete an income payment
// @Tags         distributions
// @Produce      json
// @Param        id  path  string  true  "Distribution ID"
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /distributions/{id} [delete]
//
// The record is soft-deleted, being the user's own; the realized event derived
// from it is removed outright, since a derived row has nothing to preserve.
func (h *distributionHandler) DeleteDistribution(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid distribution id"})
		return
	}

	var dist models.Distribution
	if err := h.db.First(&dist, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "distribution not found"})
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := portfolio.DeleteDistributionEvents(tx, dist.ID); err != nil {
			return err
		}
		return tx.Delete(&dist).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete distribution"})
		return
	}
	c.Status(http.StatusNoContent)
}
