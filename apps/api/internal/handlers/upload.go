package handlers

import (
	"net/http"

	"github.com/eriksalino/wealthogic/api/internal/uploads"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UploadHandler interface {
	Upload(c *gin.Context)
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
// @Param        file          formData  file    true  "File to import"
// @Param        file_type     formData  string  true  "Kind of data in the file (e.g. holdings)"
// @Param        account_type  formData  string  true  "Institution the file came from (e.g. fidelity)"
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

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open uploaded file"})
		return
	}
	defer file.Close()

	result, err := fileHandler.Process(h.db, file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}
