package handlers

import (
	"net/http"
	"strconv"

	"github.com/eriksalino/wealthogic/api/internal/marketdata"
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
	enricher *marketdata.Enricher
}

func NewUploadHandler(db *gorm.DB, enricher *marketdata.Enricher) UploadHandler {
	return &uploadHandler{db: db, registry: uploads.NewRegistry(), enricher: enricher}
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

	opts := uploads.Options{Enricher: h.enricher}
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

// uploadRecord is an Upload as history shows it: the file, plus the name of the
// account it was imported into. The id is what the row stores, but a name is
// what a reader recognizes.
type uploadRecord struct {
	models.Upload
	AccountName string `json:"account_name"`
} // @name UploadRecord

type paginatedUploads struct {
	Data     []uploadRecord `json:"data"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
} // @name PaginatedUploads

// withAccountNames pairs each upload with its account's name, looking the names
// up in one query rather than per row. An upload whose account is missing (or
// which was imported without one) keeps an empty name, which the UI renders as
// unknown rather than as a bare uuid.
func (h *uploadHandler) withAccountNames(ups []models.Upload) ([]uploadRecord, error) {
	ids := make([]uuid.UUID, 0, len(ups))
	seen := map[uuid.UUID]bool{}
	for _, u := range ups {
		if u.AccountID == uuid.Nil || seen[u.AccountID] {
			continue
		}
		seen[u.AccountID] = true
		ids = append(ids, u.AccountID)
	}

	names := map[uuid.UUID]string{}
	if len(ids) > 0 {
		var accounts []models.Account
		if err := h.db.Select("id", "account_name").Find(&accounts, "id IN ?", ids).Error; err != nil {
			return nil, err
		}
		for _, a := range accounts {
			names[a.ID] = a.AccountName
		}
	}

	records := make([]uploadRecord, 0, len(ups))
	for _, u := range ups {
		records = append(records, uploadRecord{Upload: u, AccountName: names[u.AccountID]})
	}
	return records, nil
}

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

	records, err := h.withAccountNames(uploads)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch uploads"})
		return
	}

	c.JSON(http.StatusOK, paginatedUploads{
		Data:     records,
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
// @Tags         upload-transactions
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
