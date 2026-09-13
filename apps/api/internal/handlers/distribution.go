package handlers

import (
	"net/http"
	"strconv"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DistributionHandler interface {
	GetDistributions(c *gin.Context)
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
