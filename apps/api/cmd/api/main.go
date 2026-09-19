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
	"github.com/eriksalino/wealthogic/api/internal/marketdata"
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

	// Reference data (sector, industry) comes from a market-data provider. With
	// no key configured the enricher is nil and every call through it is a
	// no-op, so the app runs unchanged without one.
	enricher := marketdata.NewEnricher(marketdata.NewFMPClient(os.Getenv("FMP_API_KEY")))
	if !enricher.Enabled() {
		log.Println("market data: FMP_API_KEY not set, holding profiles will not be fetched")
	}

	accountHandler := handlers.NewAccountHandler(database)
	holdingHandler := handlers.NewHoldingHandler(database, enricher)
	taxLotHandler := handlers.NewTaxLotHandler(database)
	transactionHandler := handlers.NewTransactionHandler(database)
	distributionHandler := handlers.NewDistributionHandler(database)
	userHandler := handlers.NewUserHandler(database)
	uploadHandler := handlers.NewUploadHandler(database, enricher)
	taxHandler := handlers.NewTaxHandler(database)

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
	r.GET("/holdings", holdingHandler.GetHoldings)
	r.POST("/holdings", holdingHandler.CreateHolding)
	r.PATCH("/holdings/:id", holdingHandler.UpdateHolding)
	r.GET("/holdings/allocation", holdingHandler.GetAllocation)
	r.POST("/holdings/backfill-profiles", holdingHandler.BackfillProfiles)
	r.GET("/tax-lots", taxLotHandler.GetTaxLots)
	r.POST("/tax-lots", taxLotHandler.CreateTaxLot)
	r.PATCH("/tax-lots/:id", taxLotHandler.UpdateTaxLot)
	r.GET("/distributions", distributionHandler.GetDistributions)
	r.POST("/distributions", distributionHandler.CreateDistribution)
	r.PATCH("/distributions/:id", distributionHandler.UpdateDistribution)
	r.DELETE("/distributions/:id", distributionHandler.DeleteDistribution)
	r.GET("/transactions", transactionHandler.GetTransactions)
	r.POST("/transactions", transactionHandler.CreateTransaction)
	r.PATCH("/transactions/:id", transactionHandler.UpdateTransaction)
	r.DELETE("/transactions/:id", transactionHandler.DeleteTransaction)
	r.GET("/users", userHandler.GetUsers)
	r.POST("/users", userHandler.CreateUser)
	r.POST("/uploads", uploadHandler.Upload)
	r.GET("/uploads", uploadHandler.GetUploads)
	r.GET("/upload-transactions", uploadHandler.GetUploadTransactions)
	r.GET("/tax/summary", taxHandler.GetSummary)
	r.GET("/tax/events", taxHandler.GetEvents)
	r.GET("/tax/jurisdictions", taxHandler.GetJurisdictions)
	r.GET("/tax/rules", taxHandler.GetRules)
	r.GET("/tax/profiles", taxHandler.GetProfiles)
	r.PUT("/tax/profiles", taxHandler.PutProfile)
	r.POST("/tax/recompute", taxHandler.Recompute)

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
