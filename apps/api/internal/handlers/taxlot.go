package handlers

import (
	"net/http"
	"strconv"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TaxLotHandler interface {
	GetTaxLots(c *gin.Context)
	CreateTaxLot(c *gin.Context)
	UpdateTaxLot(c *gin.Context)
}

type taxLotHandler struct {
	db *gorm.DB
}

func NewTaxLotHandler(db *gorm.DB) TaxLotHandler {
	return &taxLotHandler{db: db}
}

type paginatedTaxLots struct {
	Data     []models.TaxLot `json:"data"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
} // @name PaginatedTaxLots

// GetTaxLots godoc
// @Summary      List tax lots with pagination
// @Tags         tax-lots
// @Produce      json
// @Param        page        query     int     false  "Page number (default 1)"
// @Param        page_size   query     int     false  "Items per page (default 20, max 100)"
// @Param        holding_id  query     string  false  "Filter to a single holding"
// @Success      200         {object}  paginatedTaxLots
// @Failure      400         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /tax-lots [get]
func (h *taxLotHandler) GetTaxLots(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Optional holding_id filter, applied to both the count and the page query.
	applyFilter := func(q *gorm.DB) *gorm.DB { return q }
	if hid := c.Query("holding_id"); hid != "" {
		holdingID, err := uuid.Parse(hid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid holding_id"})
			return
		}
		applyFilter = func(q *gorm.DB) *gorm.DB { return q.Where("holding_id = ?", holdingID) }
	}

	var total int64
	if err := applyFilter(h.db.Model(&models.TaxLot{})).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax lots"})
		return
	}

	var lots []models.TaxLot
	offset := (page - 1) * pageSize
	// Newest first. UUIDv7 ids are time-ordered, so id breaks date ties in
	// insertion order.
	if err := applyFilter(h.db.Model(&models.TaxLot{})).Order("purchase_date DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&lots).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax lots"})
		return
	}

	c.JSON(http.StatusOK, paginatedTaxLots{
		Data:     lots,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

type createTaxLotRequest struct {
	HoldingID        uuid.UUID `json:"holding_id"`
	AccountID        uuid.UUID `json:"account_id"`
	PurchaseDate     string    `json:"purchase_date" binding:"required"`
	PurchaseQuantity float64   `json:"purchase_quantity" binding:"required"`
	PurchasePrice    float64   `json:"purchase_price" binding:"required"`
	Commission       float64   `json:"commission"`
	Fees             float64   `json:"fees"`
} // @name CreateTaxLotRequest

// taxLotWithTransaction is returned when a lot is added manually: the lot and
// the buy transaction that was recorded alongside it.
type taxLotWithTransaction struct {
	TaxLot      models.TaxLot      `json:"tax_lot"`
	Transaction models.Transaction `json:"transaction"`
} // @name TaxLotWithTransaction

// CreateTaxLot godoc
// @Summary      Add a tax lot
// @Description  Records a purchase lot against a holding and, in the same transaction, the buy transaction that produced it.
// @Tags         tax-lots
// @Accept       json
// @Produce      json
// @Param        tax_lot  body      createTaxLotRequest  true  "Tax lot payload"
// @Success      201      {object}  taxLotWithTransaction
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /tax-lots [post]
func (h *taxLotHandler) CreateTaxLot(c *gin.Context) {
	var req createTaxLotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.HoldingID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "holding_id is required"})
		return
	}
	if req.AccountID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}

	purchaseDate, err := parseInputDate(req.PurchaseDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "purchase_date must be YYYY-MM-DD or RFC3339"})
		return
	}

	var holding models.Holding
	if err := h.db.First(&holding, "id = ?", req.HoldingID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "holding not found"})
		return
	}

	var account models.Account
	if err := h.db.First(&account, "id = ?", req.AccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not found"})
		return
	}

	// The lot and its buy transaction share the holding's identity. A buy is a
	// cash outflow, so the transaction amount is negative and includes fees.
	assetType := holding.AssetType
	description := holding.Description
	qty := req.PurchaseQuantity
	price := req.PurchasePrice
	amount := -(qty*price + req.Commission + req.Fees)

	lot := models.TaxLot{
		AssetType:         assetType,
		Symbol:            holding.Symbol,
		AssetDescription:  description,
		PurchaseDate:      purchaseDate,
		PurchaseQuantity:  qty,
		PurchasePrice:     price,
		RemainingQuantity: qty,
		HoldingID:         &holding.ID,
		AccountID:         req.AccountID,
	}

	txn := models.Transaction{
		AccountID:        req.AccountID,
		HoldingID:        &holding.ID,
		AssetType:        &assetType,
		Symbol:           holding.Symbol,
		AssetDescription: &description,
		Action:           "Buy",
		Date:             purchaseDate,
		Quantity:         &qty,
		Price:            &price,
		Amount:           amount,
		Commission:       req.Commission,
		Fees:             req.Fees,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&lot).Error; err != nil {
			return err
		}
		if err := tx.Create(&txn).Error; err != nil {
			return err
		}
		// Tie the buy transaction to the lot it opened.
		if err := tx.Create(&models.TaxLotTransaction{TaxLotID: lot.ID, TransactionID: txn.ID, Quantity: qty}).Error; err != nil {
			return err
		}
		return portfolio.RecalcHolding(tx, &holding)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add tax lot"})
		return
	}

	c.JSON(http.StatusCreated, taxLotWithTransaction{TaxLot: lot, Transaction: txn})
}


type updateTaxLotRequest struct {
	AccountID        uuid.UUID `json:"account_id"`
	PurchaseDate     string    `json:"purchase_date" binding:"required"`
	PurchaseQuantity float64   `json:"purchase_quantity" binding:"required"`
	PurchasePrice    float64   `json:"purchase_price" binding:"required"`
} // @name UpdateTaxLotRequest

// UpdateTaxLot godoc
// @Summary      Update a tax lot
// @Description  Updates the lot and recomputes the holding's position from its open lots. The originally recorded buy transaction is left as-is.
// @Tags         tax-lots
// @Accept       json
// @Produce      json
// @Param        id       path      string               true  "Tax lot ID"
// @Param        tax_lot  body      updateTaxLotRequest  true  "Tax lot payload"
// @Success      200      {object}  models.TaxLot
// @Failure      400      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /tax-lots/{id} [patch]
func (h *taxLotHandler) UpdateTaxLot(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tax lot id"})
		return
	}

	var req updateTaxLotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AccountID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}

	purchaseDate, err := parseInputDate(req.PurchaseDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "purchase_date must be YYYY-MM-DD or RFC3339"})
		return
	}

	var lot models.TaxLot
	if err := h.db.First(&lot, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tax lot not found"})
		return
	}

	var account models.Account
	if err := h.db.First(&account, "id = ?", req.AccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not found"})
		return
	}

	lot.AccountID = req.AccountID
	lot.PurchaseDate = purchaseDate
	lot.PurchaseQuantity = req.PurchaseQuantity
	lot.PurchasePrice = req.PurchasePrice
	// No partial-sell concept yet, so a lot's remaining quantity tracks its
	// purchase quantity.
	lot.RemainingQuantity = req.PurchaseQuantity

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&lot).Error; err != nil {
			return err
		}
		if lot.HoldingID == nil {
			return nil
		}
		var holding models.Holding
		if err := tx.First(&holding, "id = ?", *lot.HoldingID).Error; err != nil {
			return err
		}
		return portfolio.RecalcHolding(tx, &holding)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tax lot"})
		return
	}

	c.JSON(http.StatusOK, lot)
}
