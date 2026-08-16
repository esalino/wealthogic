package handlers

import (
	"net/http"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserHandler interface {
	GetUsers(c *gin.Context)
	CreateUser(c *gin.Context)
}

type userHandler struct {
	db *gorm.DB
}

func NewUserHandler(db *gorm.DB) UserHandler {
	return &userHandler{db: db}
}

func (h *userHandler) GetUsers(c *gin.Context) {
	var users []models.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}
	c.JSON(http.StatusOK, users)
}

type createUserRequest struct {
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
} // @name CreateUserRequest

// CreateUser godoc
// @Summary      Create a user
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        user  body      createUserRequest  true  "User payload"
// @Success      201   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      500   {object}  map[string]string
// @Router       /users [post]
func (h *userHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := models.User{FirstName: req.FirstName, LastName: req.LastName}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusCreated, user)
}
