package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// A "tax lot" is now just a stock buy transaction. These handlers project buy
// transactions into the lot shape the UI expects, so the frontend is unchanged.

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

// taxLotView is a buy transaction presented as a tax lot.
type taxLotView struct {
	ID                uuid.UUID  `json:"id"`
	AssetType         string     `json:"asset_type"`
	Symbol            string     `json:"symbol"`
	AssetDescription  string     `json:"asset_description"`
	PurchaseDate      time.Time  `json:"purchase_date"`
	PurchaseQuantity  float64    `json:"purchase_quantity"`
	PurchasePrice     float64    `json:"purchase_price"`
	RemainingQuantity float64    `json:"remaining_quantity"`
	HoldingID         *uuid.UUID `json:"holding_id"`
	AccountID         uuid.UUID  `json:"account_id"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
} // @name TaxLot

func derefString(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func derefFloat(p *float64) float64 {
	if p != nil {
		return *p
	}
	return 0
}

func lotView(t models.Transaction) taxLotView {
	return taxLotView{
		ID:                t.ID,
		AssetType:         derefString(t.AssetType),
		Symbol:            t.Symbol,
		AssetDescription:  derefString(t.AssetDescription),
		PurchaseDate:      t.Date,
		PurchaseQuantity:  derefFloat(t.Quantity),
		PurchasePrice:     derefFloat(t.Price),
		RemainingQuantity: derefFloat(t.RemainingQuantity),
		HoldingID:         t.HoldingID,
		AccountID:         t.AccountID,
		CreatedAt:         t.CreatedAt,
		UpdatedAt:         t.UpdatedAt,
	}
}

type paginatedTaxLots struct {
	Data     []taxLotView `json:"data"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
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

	var holdingID *uuid.UUID
	if hid := c.Query("holding_id"); hid != "" {
		id, err := uuid.Parse(hid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid holding_id"})
			return
		}
		holdingID = &id
	}

	// Lot-forming stock buys carry a non-null remaining_quantity.
	filter := func(q *gorm.DB) *gorm.DB {
		q = q.Where("LOWER(action) = ? AND remaining_quantity IS NOT NULL", "buy")
		if holdingID != nil {
			q = q.Where("holding_id = ?", *holdingID)
		}
		return q
	}

	var total int64
	if err := filter(h.db.Model(&models.Transaction{})).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax lots"})
		return
	}

	var buys []models.Transaction
	offset := (page - 1) * pageSize
	if err := filter(h.db).Order("date DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&buys).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax lots"})
		return
	}

	views := make([]taxLotView, len(buys))
	for i := range buys {
		views[i] = lotView(buys[i])
	}

	c.JSON(http.StatusOK, paginatedTaxLots{Data: views, Total: total, Page: page, PageSize: pageSize})
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

// taxLotWithTransaction is returned when a lot is added: the lot view and the
// underlying buy transaction (they are the same row).
type taxLotWithTransaction struct {
	TaxLot      taxLotView         `json:"tax_lot"`
	Transaction models.Transaction `json:"transaction"`
} // @name TaxLotWithTransaction

// CreateTaxLot godoc
// @Summary      Add a tax lot
// @Description  Records a stock buy against a holding. The buy transaction is the tax lot.
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

	assetType := holding.AssetType
	description := holding.Description
	qty := req.PurchaseQuantity
	price := req.PurchasePrice
	amount := -(qty*price + req.Commission + req.Fees)

	txn := models.Transaction{
		AccountID:         req.AccountID,
		HoldingID:         &holding.ID,
		AssetType:         &assetType,
		Symbol:            holding.Symbol,
		AssetDescription:  &description,
		Action:            "Buy",
		Date:              purchaseDate,
		Quantity:          &qty,
		Price:             &price,
		Amount:            amount,
		Commission:        req.Commission,
		Fees:              req.Fees,
		RemainingQuantity: &qty,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&txn).Error; err != nil {
			return err
		}
		return portfolio.RecalcHolding(tx, &holding)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add tax lot"})
		return
	}

	c.JSON(http.StatusCreated, taxLotWithTransaction{TaxLot: lotView(txn), Transaction: txn})
}

type updateTaxLotRequest struct {
	AccountID        uuid.UUID `json:"account_id"`
	PurchaseDate     string    `json:"purchase_date" binding:"required"`
	PurchaseQuantity float64   `json:"purchase_quantity" binding:"required"`
	PurchasePrice    float64   `json:"purchase_price" binding:"required"`
} // @name UpdateTaxLotRequest

// UpdateTaxLot godoc
// @Summary      Update a tax lot
// @Description  Updates the buy transaction behind the lot and recomputes the holding. Rejected if the lot has already been sold from.
// @Tags         tax-lots
// @Accept       json
// @Produce      json
// @Param        id       path      string               true  "Tax lot (buy transaction) ID"
// @Param        tax_lot  body      updateTaxLotRequest  true  "Tax lot payload"
// @Success      200      {object}  taxLotView
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

	var txn models.Transaction
	if err := h.db.Where("id = ? AND LOWER(action) = ?", id, "buy").First(&txn).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tax lot not found"})
		return
	}

	disposed, err := buyHasDisposals(h.db, txn.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tax lot"})
		return
	}
	if disposed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot edit a lot that has already been sold from"})
		return
	}

	var account models.Account
	if err := h.db.First(&account, "id = ?", req.AccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not found"})
		return
	}

	qty := req.PurchaseQuantity
	price := req.PurchasePrice
	txn.AccountID = req.AccountID
	txn.Date = purchaseDate
	txn.Quantity = &qty
	txn.Price = &price
	txn.RemainingQuantity = &qty
	txn.Amount = -(qty*price + txn.Commission + txn.Fees)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&txn).Error; err != nil {
			return err
		}
		if txn.HoldingID != nil {
			return rebuildHolding(tx, *txn.HoldingID, true)
		}
		return nil
	})
	if errors.Is(err, errInsufficientShares) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not enough shares to cover existing sells"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tax lot"})
		return
	}

	c.JSON(http.StatusOK, lotView(txn))
}
