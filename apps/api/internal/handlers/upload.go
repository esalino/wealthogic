package handlers

import (
	"net/http"
	"strconv"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/uploads"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UploadHandler interface {
	Upload(c *gin.Context)
	GetUploads(c *gin.Context)
	GetUploadTransactions(c *gin.Context)
}

type uploadHandler struct {
	db       *gorm.DB
	registry *uploads.Registry
}

func NewUploadHandler(db *gorm.DB) UploadHandler {
	return &uploadHandler{db: db, registry: uploads.NewRegistry()}
}

// Upload godoc
// @Summary      Upload a data file for import
// @Description  Routes the file to a handler selected by file_type + account_type (currently: holdings + fidelity)
// @Tags         uploads
// @Accept       multipart/form-data
// @Produce      json
// @Param        file          formData  file    true   "File to import"
// @Param        file_type     formData  string  true   "Kind of data in the file (e.g. holdings, transactions)"
// @Param        account_type  formData  string  true   "Institution the file came from (e.g. fidelity)"
// @Param        account_id    formData  string  false  "Account to tie the data to (required for transactions)"
// @Success      200  {object}  uploads.Result
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /uploads [post]
func (h *uploadHandler) Upload(c *gin.Context) {
	fileType := c.PostForm("file_type")
	if fileType == "" {
		fileType = c.Query("file_type")
	}
	accountType := c.PostForm("account_type")
	if accountType == "" {
		accountType = c.Query("account_type")
	}
	if fileType == "" || accountType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_type and account_type are required"})
		return
	}

	fileHandler, ok := h.registry.Get(fileType, accountType)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no handler for file_type " + fileType + " and account_type " + accountType})
		return
	}

	var opts uploads.Options
	if accountID := c.PostForm("account_id"); accountID != "" {
		id, err := uuid.Parse(accountID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "account_id must be a valid uuid"})
			return
		}
		opts.AccountID = id
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	opts.FileName = fileHeader.Filename

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open uploaded file"})
		return
	}
	defer file.Close()

	result, err := fileHandler.Process(h.db, file, opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

type paginatedUploads struct {
	Data     []models.Upload `json:"data"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
} // @name PaginatedUploads

// GetUploads godoc
// @Summary      List uploads with pagination
// @Tags         uploads
// @Produce      json
// @Param        page       query     int  false  "Page number (default 1)"
// @Param        page_size  query     int  false  "Items per page (default 20, max 100)"
// @Success      200        {object}  paginatedUploads
// @Failure      500        {object}  map[string]string
// @Router       /uploads [get]
func (h *uploadHandler) GetUploads(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	if err := h.db.Model(&models.Upload{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch uploads"})
		return
	}

	var uploads []models.Upload
	offset := (page - 1) * pageSize
	// Newest upload first.
	if err := h.db.Order("created_at DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&uploads).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch uploads"})
		return
	}

	c.JSON(http.StatusOK, paginatedUploads{
		Data:     uploads,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

type paginatedUploadTransactions struct {
	Data     []models.UploadTransaction `json:"data"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
} // @name PaginatedUploadTransactions

// GetUploadTransactions godoc
// @Summary      List upload transactions with pagination
// @Tags         uploads
// @Produce      json
// @Param        page       query     int  false  "Page number (default 1)"
// @Param        page_size  query     int  false  "Items per page (default 20, max 100)"
// @Success      200        {object}  paginatedUploadTransactions
// @Failure      500        {object}  map[string]string
// @Router       /upload-transactions [get]
func (h *uploadHandler) GetUploadTransactions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var total int64
	if err := h.db.Model(&models.UploadTransaction{}).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch upload transactions"})
		return
	}

	var txns []models.UploadTransaction
	offset := (page - 1) * pageSize
	// Newest first, matching the transactions list.
	if err := h.db.Order("date DESC").Order("id DESC").Offset(offset).Limit(pageSize).Find(&txns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch upload transactions"})
		return
	}

	c.JSON(http.StatusOK, paginatedUploadTransactions{
		Data:     txns,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}
