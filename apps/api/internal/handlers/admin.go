package handlers

import (
	"net/http"

	"github.com/eriksalino/wealthogic/api/internal/adminlog"
	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// latestLogs is the latest-runs response. Declared here rather than returning
// the slice inline so the generated API docs can resolve the model type from
// this file's imports.
type latestLogs []models.AdminLog // @name LatestAdminLogs

type AdminHandler interface {
	GetLatestLogs(c *gin.Context)
}

type adminHandler struct {
	db *gorm.DB
}

func NewAdminHandler(db *gorm.DB) AdminHandler {
	return &adminHandler{db: db}
}

// GetLatestLogs godoc
// @Summary      The most recent run of each maintenance utility
// @Tags         admin
// @Produce      json
// @Success      200  {object}  latestLogs
// @Failure      500  {object}  map[string]string
// @Router       /admin/logs/latest [get]
//
// One row per utility, which is what the Admin page needs to put a "last run"
// line on each tool - so the whole page costs one query rather than one per
// card. A utility that has never run is simply absent.
func (h *adminHandler) GetLatestLogs(c *gin.Context) {
	var logs latestLogs
	logs, err := adminlog.LatestPerUtility(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch admin logs"})
		return
	}
	c.JSON(http.StatusOK, logs)
}
