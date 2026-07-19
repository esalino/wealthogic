// @title           My Portfolio API
// @version         1.0
// @description     Portfolio tracking API
// @host            localhost:8080
// @BasePath        /
package main

import (
	"log"
	"net/http"
	"os"

	_ "github.com/eriksalino/wealthogic/api/docs"
	"github.com/eriksalino/wealthogic/api/internal/db"
	"github.com/eriksalino/wealthogic/api/internal/handlers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using environment variables")
	}

	database, err := db.Connect()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	accountHandler := handlers.NewAccountHandler(database)
	userHandler := handlers.NewUserHandler(database)
	uploadHandler := handlers.NewUploadHandler(database)

	r := gin.Default()

	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		log.Fatalf("failed to connect to database: %v", err)
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{corsOrigin},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		AllowCredentials: true,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/accounts", accountHandler.GetAccounts)
	r.POST("/accounts", accountHandler.CreateAccount)
	r.PATCH("/accounts/:id", accountHandler.UpdateAccount)
	r.GET("/users", userHandler.GetUsers)
	r.POST("/uploads", uploadHandler.Upload)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	log.Printf("API server listening on %s", addr)
	log.Printf("Swagger UI available at http://localhost%s/swagger/index.html", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
