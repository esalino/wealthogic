package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/eriksalino/wealthogic/api/internal/tax"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionHandler interface {
	GetTransactions(c *gin.Context)
	CreateTransaction(c *gin.Context)
	UpdateTransaction(c *gin.Context)
	DeleteTransaction(c *gin.Context)
}

type transactionHandler struct {
	db *gorm.DB
}

func NewTransactionHandler(db *gorm.DB) TransactionHandler {
	return &transactionHandler{db: db}
}

type paginatedTransactions struct {
	Data     []models.Transaction `json:"data"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
} // @name PaginatedTransactions

// GetTransactions godoc
// @Summary      List transactions with pagination
// @Tags         transactions
// @Produce      json
// @Param        page        query     int     false  "Page number (default 1)"
// @Param        page_size   query     int     false  "Items per page (default 20, max 100)"
// @Param        holding_id  query     string  false  "Filter to a single holding"
// @Success      200         {object}  paginatedTransactions
// @Failure      400         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /transactions [get]
func (h *transactionHandler) GetTransactions(c *gin.Context) {
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
	if err := applyFilter(h.db.Model(&models.Transaction{})).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch transactions"})
		return
	}

	var transactions []models.Transaction
	offset := (page - 1) * pageSize
	// Newest first. UUIDv7 ids are time-ordered, so id breaks date ties in
	// insertion order.
	if err := applyFilter(h.db.Model(&models.Transaction{})).Order("date DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&transactions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch transactions"})
		return
	}

	c.JSON(http.StatusOK, paginatedTransactions{
		Data:     transactions,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

type createTransactionRequest struct {
	AccountID        uuid.UUID  `json:"account_id"`
	HoldingID        *uuid.UUID `json:"holding_id"`
	AssetType        *string    `json:"asset_type"`
	Symbol           string     `json:"symbol"`
	AssetDescription *string    `json:"asset_description"`
	Action           string     `json:"action" binding:"required"`
	Date             string     `json:"date" binding:"required"`
	Quantity         *float64   `json:"quantity"`
	Price            *float64   `json:"price"`
	Amount           float64    `json:"amount"`
	Commission       float64    `json:"commission"`
	Fees             float64    `json:"fees"`
	SettlementDate   *string    `json:"settlement_date"`
	RealizedGains    float64    `json:"realized_gains"`
} // @name CreateTransactionRequest

// CreateTransaction godoc
// @Summary      Create a transaction
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Param        transaction  body      createTransactionRequest  true  "Transaction payload"
// @Success      201          {object}  models.Transaction
// @Failure      400          {object}  map[string]string
// @Failure      500          {object}  map[string]string
// @Router       /transactions [post]
func (h *transactionHandler) CreateTransaction(c *gin.Context) {
	var req createTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AccountID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}

	date, err := parseInputDate(req.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD or RFC3339"})
		return
	}

	var settlementDate *time.Time
	if req.SettlementDate != nil && *req.SettlementDate != "" {
		sd, err := parseInputDate(*req.SettlementDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "settlement_date must be YYYY-MM-DD or RFC3339"})
			return
		}
		settlementDate = &sd
	}

	var account models.Account
	if err := h.db.First(&account, "id = ?", req.AccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not found"})
		return
	}

	if req.HoldingID != nil {
		var holding models.Holding
		if err := h.db.First(&holding, "id = ?", *req.HoldingID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "holding not found"})
			return
		}
	}

	txn := models.Transaction{
		AccountID:        req.AccountID,
		HoldingID:        req.HoldingID,
		AssetType:        req.AssetType,
		Symbol:           req.Symbol,
		AssetDescription: req.AssetDescription,
		Action:           req.Action,
		Date:             date,
		Quantity:         req.Quantity,
		Price:            req.Price,
		Amount:           req.Amount,
		Commission:       req.Commission,
		Fees:             req.Fees,
		SettlementDate:   settlementDate,
		RealizedGains:    req.RealizedGains,
	}

	// Buys and sells against a holding touch tax lots: a buy opens a new lot; a
	// sell depletes open lots. Both need a holding plus a quantity and price;
	// other actions are just recorded. (Only buy/sell for now.)
	isBuy := strings.EqualFold(req.Action, "Buy") && req.HoldingID != nil && req.Quantity != nil && req.Price != nil
	isSell := strings.EqualFold(req.Action, "Sell") && req.HoldingID != nil && req.Quantity != nil && req.Price != nil

	// A hand-entered trade is the ordinary long case: a buy opens a lot, a sell
	// closes one. The importer sets these too, from Fidelity's opening/closing
	// markers - options are the only thing that needs the distinction spelled
	// out, and they aren't entered by hand yet.
	if isBuy {
		txn.Effect, txn.Direction = models.EffectOpen, models.DirectionLong
	} else if isSell {
		txn.Effect, txn.Direction = models.EffectClose, models.DirectionLong
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		var holding models.Holding
		if isBuy || isSell {
			if err := tx.First(&holding, "id = ?", *req.HoldingID).Error; err != nil {
				return err
			}
		}

		if err := tx.Create(&txn).Error; err != nil {
			return err
		}

		// A buy opens a tax lot of its own.
		if isBuy {
			if err := portfolio.OpenLot(tx, &txn); err != nil {
				return err
			}
		}

		// A sell consumes shares from open buy lots (FIFO/LIFO per the account),
		// writing a Gain row per lot and recording the realized total. Selling
		// more than is held is rejected.
		if isSell {
			applier, err := tax.NewApplier(tx)
			if err != nil {
				return err
			}
			realized, unfilled, err := portfolio.DepleteLots(tx, &txn, account.DefaultCostBasis, applier)
			if err != nil {
				return err
			}
			if unfilled > 0 {
				return portfolio.ErrInsufficientShares
			}
			txn.RealizedGains = realized
			if err := tx.Save(&txn).Error; err != nil {
				return err
			}
		}

		if isBuy || isSell {
			return portfolio.RecalcHolding(tx, &holding)
		}
		return nil
	})
	if errors.Is(err, portfolio.ErrInsufficientShares) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not enough shares to sell"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create transaction"})
		return
	}

	c.JSON(http.StatusCreated, txn)
}

type updateTransactionRequest struct {
	AccountID  uuid.UUID `json:"account_id"`
	Action     string    `json:"action" binding:"required"`
	Date       string    `json:"date" binding:"required"`
	Quantity   *float64  `json:"quantity"`
	Price      *float64  `json:"price"`
	Amount     float64   `json:"amount"`
	Commission float64   `json:"commission"`
	Fees       float64   `json:"fees"`
} // @name UpdateTransactionRequest

// UpdateTransaction godoc
// @Summary      Update a transaction
// @Description  Updates the transaction; if it belongs to a holding, the holding's tax lots and aggregates are rebuilt from its trades, including the lot this transaction opened.
// @Tags         transactions
// @Accept       json
// @Produce      json
// @Param        id           path      string                    true  "Transaction ID"
// @Param        transaction  body      updateTransactionRequest  true  "Transaction payload"
// @Success      200          {object}  models.Transaction
// @Failure      400          {object}  map[string]string
// @Failure      404          {object}  map[string]string
// @Failure      500          {object}  map[string]string
// @Router       /transactions/{id} [patch]
func (h *transactionHandler) UpdateTransaction(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction id"})
		return
	}

	var req updateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AccountID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}

	date, err := parseInputDate(req.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD or RFC3339"})
		return
	}

	var txn models.Transaction
	if err := h.db.First(&txn, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "transaction not found"})
		return
	}

	var account models.Account
	if err := h.db.First(&account, "id = ?", req.AccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not found"})
		return
	}

	// A buy opens a tax lot; editing one whose shares have already been
	// sold would corrupt recorded gains. Reject it (revisit later).
	if strings.EqualFold(txn.Action, "Buy") {
		disposed, err := buyHasDisposals(h.db, txn.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update transaction"})
			return
		}
		if disposed {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot edit a buy that has already been sold from"})
			return
		}
	}

	// Only the fields the edit form owns are touched; holding_id, symbol,
	// settlement_date, realized_gains, etc. are preserved.
	txn.AccountID = req.AccountID
	txn.Action = req.Action
	txn.Date = date
	txn.Quantity = req.Quantity
	txn.Price = req.Price
	txn.Amount = req.Amount
	txn.Commission = req.Commission
	txn.Fees = req.Fees

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&txn).Error; err != nil {
			return err
		}
		// A sell's lot depletion can't be reversed in isolation, so rebuild the
		// whole holding from its buys and sells after any change. Strict so an
		// edit that over-sells is rejected.
		if txn.HoldingID != nil {
			return portfolio.RebuildHolding(tx, *txn.HoldingID, true)
		}
		return nil
	}); err != nil {
		if errors.Is(err, portfolio.ErrInsufficientShares) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "not enough shares to sell"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update transaction"})
		return
	}

	// The rebuild may have recomputed this sell's realized gain, so reload.
	if err := h.db.First(&txn, "id = ?", txn.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load transaction"})
		return
	}

	c.JSON(http.StatusOK, txn)
}

// DeleteTransaction godoc
// @Summary      Delete a transaction
// @Description  Soft-deletes the transaction; if it belongs to a holding, the holding's tax lots and aggregates are rebuilt from its remaining trades, which removes the lot a deleted buy opened.
// @Tags         transactions
// @Produce      json
// @Param        id   path      string  true  "Transaction ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /transactions/{id} [delete]
func (h *transactionHandler) DeleteTransaction(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid transaction id"})
		return
	}

	var txn models.Transaction
	if err := h.db.First(&txn, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "transaction not found"})
		return
	}

	// A buy that's been sold from can't be removed without corrupting gains.
	if strings.EqualFold(txn.Action, "Buy") {
		disposed, err := buyHasDisposals(h.db, txn.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete transaction"})
			return
		}
		if disposed {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete a buy that has already been sold from"})
			return
		}
	}

	// Deleting a sell frees shares (lenient rebuild); deleting a buy removes
	// shares, so the rebuild is strict and rejects if it would over-sell.
	strict := !strings.EqualFold(txn.Action, "Sell")

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&txn).Error; err != nil {
			return err
		}
		if txn.HoldingID != nil {
			return portfolio.RebuildHolding(tx, *txn.HoldingID, strict)
		}
		return nil
	}); err != nil {
		if errors.Is(err, portfolio.ErrInsufficientShares) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete: shares are needed to cover existing sells"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete transaction"})
		return
	}

	c.Status(http.StatusNoContent)
}

// buyHasDisposals reports whether any realized event draws from the lot this
// buy opened, i.e. shares of it have been sold.
func buyHasDisposals(db *gorm.DB, buyID uuid.UUID) (bool, error) {
	var count int64
	err := db.Model(&models.RealizedEvent{}).
		Joins("JOIN tax_lots ON tax_lots.id = realized_events.tax_lot_id").
		Where("tax_lots.opening_transaction_id = ?", buyID).
		Count(&count).Error
	return count > 0, err
}
