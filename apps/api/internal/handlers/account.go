package handlers

import (
	"net/http"
	"strconv"

	"github.com/eriksalino/my-portfolio/api/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AccountHandler interface {
	CreateAccount(c *gin.Context)
	GetAccounts(c *gin.Context)
}

type accountHandler struct {
	db *gorm.DB
}

func NewAccountHandler(db *gorm.DB) AccountHandler {
	return &accountHandler{db: db}
}

type createAccountRequest struct {
	AccountName string  `json:"account_name" binding:"required"`
	AccountType string  `json:"account_type" binding:"required"`
	Balance     float64 `json:"balance"`
}

type paginatedAccounts struct {
	Data     []models.Account `json:"data"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// CreateAccount godoc
// @Summary      Create a new account
// @Tags         accounts
// @Accept       json
// @Produce      json
// @Param        account  body      createAccountRequest  true  "Account payload"
// @Success      201      {object}  models.Account
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /accounts [post]
func (h *accountHandler) CreateAccount(c *gin.Context) {
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account := models.Account{
		AccountName: req.AccountName,
		AccountType: req.AccountType,
		Balance:     req.Balance,
	}

	if err := h.db.Create(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create account"})
		return
	}

	c.JSON(http.StatusCreated, account)
}

// GetAccounts godoc
// @Summary      List accounts with pagination
// @Tags         accounts
// @Produce      json
// @Param        page       query     int  false  "Page number (default 1)"
// @Param        page_size  query     int  false  "Items per page (default 20, max 100)"
// @Success      200        {object}  paginatedAccounts
// @Failure      500        {object}  map[string]string
// @Router       /accounts [get]
func (h *accountHandler) GetAccounts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	if err := h.db.Model(&models.Account{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch accounts"})
		return
	}

	var accounts []models.Account
	offset := (page - 1) * pageSize
	if err := h.db.Offset(offset).Limit(pageSize).Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch accounts"})
		return
	}

	c.JSON(http.StatusOK, paginatedAccounts{
		Data:     accounts,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}
