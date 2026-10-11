package handlers

import (
	"errors"
	"net/http"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Splits are recorded by hand for now. The Fidelity export reports one as a
// pair of REVERSE SPLIT rows keyed by CUSIP, often with the symbol blank, which
// the import can't yet tie back to a holding.

type SplitHandler interface {
	GetSplits(c *gin.Context)
	CreateSplit(c *gin.Context)
	DeleteSplit(c *gin.Context)
}

type splitHandler struct {
	db *gorm.DB
}

func NewSplitHandler(db *gorm.DB) SplitHandler {
	return &splitHandler{db: db}
}

// splitList is the splits response. Declared rather than returning the slice
// inline so the generated API docs can resolve the model type.
type splitList []models.StockSplit // @name StockSplits

// GetSplits godoc
// @Summary      List a holding's stock splits, oldest first
// @Tags         splits
// @Produce      json
// @Param        holding_id  query     string  true  "Holding ID"
// @Success      200         {object}  splitList
// @Failure      400         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /splits [get]
func (h *splitHandler) GetSplits(c *gin.Context) {
	holdingID, err := uuid.Parse(c.Query("holding_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "holding_id is required"})
		return
	}
	splits, err := portfolio.LoadSplits(h.db, holdingID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch splits"})
		return
	}
	c.JSON(http.StatusOK, splitList(splits))
}

type createSplitRequest struct {
	HoldingID     uuid.UUID `json:"holding_id"`
	EffectiveDate string    `json:"effective_date" binding:"required"`
	OldShares     float64   `json:"old_shares" binding:"required"`
	NewShares     float64   `json:"new_shares" binding:"required"`
} // @name CreateSplitRequest

// CreateSplit godoc
// @Summary      Record a stock split
// @Description  Every old_shares held before effective_date become new_shares (2-for-1 is 1 -> 2; a 1-for-8 reverse split is 8 -> 1). The holding's lots are rebuilt around it; its transactions are left as traded.
// @Tags         splits
// @Accept       json
// @Produce      json
// @Param        split  body      createSplitRequest  true  "Split payload"
// @Success      201    {object}  models.StockSplit
// @Failure      400    {object}  map[string]string
// @Failure      500    {object}  map[string]string
// @Router       /splits [post]
func (h *splitHandler) CreateSplit(c *gin.Context) {
	var req createSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.HoldingID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "holding_id is required"})
		return
	}
	if req.OldShares <= 0 || req.NewShares <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old_shares and new_shares must be positive"})
		return
	}
	date, err := parseInputDate(req.EffectiveDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "effective_date must be YYYY-MM-DD or RFC3339"})
		return
	}

	var holding models.Holding
	if err := h.db.First(&holding, "id = ?", req.HoldingID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "holding not found"})
		return
	}

	split := models.StockSplit{
		HoldingID:     holding.ID,
		EffectiveDate: date,
		OldShares:     req.OldShares,
		NewShares:     req.NewShares,
	}
	// Lenient: a missing split is usually why later sells couldn't be filled,
	// so recording one only ever repairs the ledger.
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&split).Error; err != nil {
			return err
		}
		return portfolio.RebuildHolding(tx, holding.ID, false)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record split"})
		return
	}
	c.JSON(http.StatusCreated, split)
}

// DeleteSplit godoc
// @Summary      Delete a stock split
// @Description  Rebuilds the holding's lots without it. Rejected if post-split sells would no longer be covered.
// @Tags         splits
// @Param        id   path      string  true  "Split ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /splits/{id} [delete]
func (h *splitHandler) DeleteSplit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid split id"})
		return
	}
	var split models.StockSplit
	if err := h.db.First(&split, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "split not found"})
		return
	}

	// Strict: undoing a forward split shrinks every earlier lot, which can
	// leave later sells without the shares they were filled from.
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&split).Error; err != nil {
			return err
		}
		return portfolio.RebuildHolding(tx, split.HoldingID, true)
	})
	if errors.Is(err, portfolio.ErrInsufficientShares) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete: later sells depend on the shares this split created"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete split"})
		return
	}
	c.Status(http.StatusNoContent)
}
