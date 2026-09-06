package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// errInsufficientShares is returned from the create transaction when a sell
// asks for more shares than the account's open lots hold. It maps to a 400.
var errInsufficientShares = errors.New("not enough shares to sell")

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

	// A stock buy doubles as a tax lot, so seed its open (remaining) quantity.
	if isBuy {
		q := *req.Quantity
		txn.RemainingQuantity = &q
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

		// A sell consumes shares from open buy lots (FIFO/LIFO per the account),
		// writing a Gain row per lot and recording the realized total. Selling
		// more than is held is rejected.
		if isSell {
			realized, unfilled, err := portfolio.DepleteLots(tx, &txn, account.DefaultCostBasis)
			if err != nil {
				return err
			}
			if unfilled > 0 {
				return errInsufficientShares
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
	if errors.Is(err, errInsufficientShares) {
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
// @Description  Updates the transaction; if it belongs to a holding, the holding's lots and aggregates are rebuilt from its buys and sells. A buy's own lot is not changed.
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

	// A buy doubles as a tax lot; editing one whose shares have already been
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
			return rebuildHolding(tx, *txn.HoldingID, true)
		}
		return nil
	}); err != nil {
		if errors.Is(err, errInsufficientShares) {
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
// @Description  Soft-deletes the transaction; if it belongs to a holding, the holding's lots and aggregates are rebuilt from its remaining buys and sells. A buy's own lot is not removed.
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
			return rebuildHolding(tx, *txn.HoldingID, strict)
		}
		return nil
	}); err != nil {
		if errors.Is(err, errInsufficientShares) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete: shares are needed to cover existing sells"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete transaction"})
		return
	}

	c.Status(http.StatusNoContent)
}

// buyHasDisposals reports whether any Gain row draws from this buy lot, i.e.
// shares of it have been sold.
func buyHasDisposals(db *gorm.DB, buyID uuid.UUID) (bool, error) {
	var count int64
	err := db.Model(&models.Gain{}).Where("lot_transaction_id = ?", buyID).Count(&count).Error
	return count > 0, err
}

// rebuildHolding recomputes a holding's lot remaining quantities, per-sell
// realized gains, Gain ledger rows, and aggregates by replaying its sells
// against its buy lots from scratch. A single sell's depletion can't be reversed
// in isolation, so any create/edit/delete of a holding's buy or sell rebuilds
// the whole holding.
//
// When strict is set (edits), a sell that can't be fully filled from available
// shares returns errInsufficientShares so the caller can reject it. Deletes of a
// sell pass strict=false: removing a sell only frees shares, so it must never
// fail.
func rebuildHolding(tx *gorm.DB, holdingID uuid.UUID, strict bool) error {
	// Reset every stock buy lot to its full purchased quantity.
	if err := tx.Model(&models.Transaction{}).
		Where("holding_id = ? AND LOWER(action) = ? AND remaining_quantity IS NOT NULL", holdingID, "buy").
		Update("remaining_quantity", gorm.Expr("quantity")).Error; err != nil {
		return err
	}

	// Clear this holding's capital-gain ledger; the replay recreates it. Keyed
	// by holding_id, so it also drops rows from sells that were soft-deleted.
	if err := tx.Where("holding_id = ? AND category = ?", holdingID, "capital_gain").Delete(&models.Gain{}).Error; err != nil {
		return err
	}

	// Replay sells oldest first so lots match to sells chronologically.
	var sells []models.Transaction
	if err := tx.Where("holding_id = ? AND LOWER(action) = ?", holdingID, "sell").
		Order("date ASC").Order("id ASC").Find(&sells).Error; err != nil {
		return err
	}

	costBasisMethod := map[uuid.UUID]string{}
	for i := range sells {
		sell := &sells[i]
		var realized float64
		if sell.Quantity != nil && sell.Price != nil {
			method, ok := costBasisMethod[sell.AccountID]
			if !ok {
				var account models.Account
				if err := tx.First(&account, "id = ?", sell.AccountID).Error; err == nil {
					method = account.DefaultCostBasis
				}
				costBasisMethod[sell.AccountID] = method
			}
			r, unfilled, err := portfolio.DepleteLots(tx, sell, method)
			if err != nil {
				return err
			}
			// Strict callers (edits) reject a sell that can't be fully filled
			// from the available shares.
			if strict && unfilled > 1e-9 {
				return errInsufficientShares
			}
			realized = r
		}
		sell.RealizedGains = realized
		if err := tx.Save(sell).Error; err != nil {
			return err
		}
	}

	var holding models.Holding
	if err := tx.First(&holding, "id = ?", holdingID).Error; err != nil {
		return err
	}
	return portfolio.RecalcHolding(tx, &holding)
}
