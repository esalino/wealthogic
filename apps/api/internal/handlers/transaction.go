package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TransactionHandler interface {
	GetTransactions(c *gin.Context)
	CreateTransaction(c *gin.Context)
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
// @Param        page       query     int  false  "Page number (default 1)"
// @Param        page_size  query     int  false  "Items per page (default 20, max 100)"
// @Success      200        {object}  paginatedTransactions
// @Failure      500        {object}  map[string]string
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

	var total int64
	if err := h.db.Model(&models.Transaction{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch transactions"})
		return
	}

	var transactions []models.Transaction
	offset := (page - 1) * pageSize
	// Newest first. UUIDv7 ids are time-ordered, so id breaks date ties in
	// insertion order.
	if err := h.db.Order("date DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&transactions).Error; err != nil {
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

	if err := h.db.Create(&txn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create transaction"})
		return
	}

	c.JSON(http.StatusCreated, txn)
}
