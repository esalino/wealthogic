package handlers

import (
	"net/http"
	"strconv"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GainHandler interface {
	GetGains(c *gin.Context)
}

type gainHandler struct {
	db *gorm.DB
}

func NewGainHandler(db *gorm.DB) GainHandler {
	return &gainHandler{db: db}
}

// gainSummary totals a year's realized gains, split by holding period. Used for
// the Tax Center summary tiles (independent of pagination).
type gainSummary struct {
	Total     float64 `json:"total"`
	ShortTerm float64 `json:"short_term"`
	LongTerm  float64 `json:"long_term"`
} // @name GainSummary

type paginatedGains struct {
	Data     []models.Gain `json:"data"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Summary  gainSummary   `json:"summary"`
} // @name PaginatedGains

// GetGains godoc
// @Summary      List realized gains with pagination and a year summary
// @Tags         gains
// @Produce      json
// @Param        year       query     int  false  "Filter to the tax year the gain was realized"
// @Param        page       query     int  false  "Page number (default 1)"
// @Param        page_size  query     int  false  "Items per page (default 20, max 100)"
// @Success      200        {object}  paginatedGains
// @Failure      500        {object}  map[string]string
// @Router       /gains [get]
func (h *gainHandler) GetGains(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Capital gains only for now; optional tax-year filter on the realized date.
	filter := func(q *gorm.DB) *gorm.DB {
		q = q.Where("category = ?", "capital_gain")
		if y := c.Query("year"); y != "" {
			if year, err := strconv.Atoi(y); err == nil {
				q = q.Where("EXTRACT(YEAR FROM realized_date) = ?", year)
			}
		}
		return q
	}

	var total int64
	if err := filter(h.db.Model(&models.Gain{})).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch gains"})
		return
	}

	// Year totals across all matching rows (not just the page).
	var summary gainSummary
	if err := filter(h.db.Model(&models.Gain{})).
		Select("COALESCE(SUM(amount), 0) AS total, " +
			"COALESCE(SUM(amount) FILTER (WHERE term = 'short'), 0) AS short_term, " +
			"COALESCE(SUM(amount) FILTER (WHERE term = 'long'), 0) AS long_term").
		Scan(&summary).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize gains"})
		return
	}

	var gains []models.Gain
	offset := (page - 1) * pageSize
	if err := filter(h.db).Order("realized_date DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&gains).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch gains"})
		return
	}

	if err := attachTreatments(h.db, gains); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax treatments"})
		return
	}

	c.JSON(http.StatusOK, paginatedGains{
		Data:     gains,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Summary:  summary,
	})
}

// attachTreatments loads the per-jurisdiction tax treatments for a page of
// gains in one query. The treatment table is keyed by (source_type, source_id)
// so it can serve both ledgers, which rules out a GORM association - hence the
// explicit join here rather than a Preload.
func attachTreatments(db *gorm.DB, gains []models.Gain) error {
	if len(gains) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(gains))
	for _, g := range gains {
		ids = append(ids, g.ID)
	}

	var treatments []models.TaxTreatment
	if err := db.Where("source_type = ? AND source_id IN ?", models.TaxSourceGain, ids).
		Order("jurisdiction_code ASC").Find(&treatments).Error; err != nil {
		return err
	}

	bySource := map[uuid.UUID][]models.TaxTreatment{}
	for _, t := range treatments {
		bySource[t.SourceID] = append(bySource[t.SourceID], t)
	}
	for i := range gains {
		gains[i].Treatments = bySource[gains[i].ID]
	}
	return nil
}
