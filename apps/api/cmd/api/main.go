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

	// Reference data (sector, industry) always comes from FMP - it's the vendor
	// that describes companies. With no key the provider is nil, which makes the
	// enricher nil and every call through it a no-op, so the app runs unchanged
	// without one.
	enricher := marketdata.NewEnricher(marketdata.NewFMPClient(os.Getenv("FMP_API_KEY")))
	if !enricher.Enabled() {
		log.Println("market data: FMP_API_KEY not set, holding profiles will not be fetched")
	}

	// Quotes are a separate choice, because the free tiers differ in what they
	// cover: FMP answers 402 Payment Required for symbols off the major
	// exchanges, which marketdata.app prices fine. QUOTE_PROVIDER picks one by
	// name; left empty it uses whichever has a key.
	pricer := marketdata.NewPricer(marketdata.NewQuoteProvider(marketdata.QuoteConfig{
		Provider:         os.Getenv("QUOTE_PROVIDER"),
		FMPAPIKey:        os.Getenv("FMP_API_KEY"),
		MarketDataAPIKey: os.Getenv("MARKETDATA_API_KEY"),
	}))
	if pricer.Enabled() {
		log.Printf("market data: quotes from %s", pricer.ProviderName())
	} else {
		log.Println("market data: no quote provider configured, prices will not be refreshed")
	}

	accountHandler := handlers.NewAccountHandler(database)
	holdingHandler := handlers.NewHoldingHandler(database, enricher, pricer)
	taxLotHandler := handlers.NewTaxLotHandler(database)
	splitHandler := handlers.NewSplitHandler(database)
	transactionHandler := handlers.NewTransactionHandler(database)
	distributionHandler := handlers.NewDistributionHandler(database)
	userHandler := handlers.NewUserHandler(database)
	uploadHandler := handlers.NewUploadHandler(database, enricher)
	taxHandler := handlers.NewTaxHandler(database)
	adminHandler := handlers.NewAdminHandler(database)

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
	r.POST("/holdings/refresh-prices", holdingHandler.RefreshPrices)
	r.POST("/holdings/recalculate", holdingHandler.Recalculate)
	r.GET("/tax-lots", taxLotHandler.GetTaxLots)
	r.POST("/tax-lots", taxLotHandler.CreateTaxLot)
	r.PATCH("/tax-lots/:id", taxLotHandler.UpdateTaxLot)
	r.GET("/splits", splitHandler.GetSplits)
	r.POST("/splits", splitHandler.CreateSplit)
	r.DELETE("/splits/:id", splitHandler.DeleteSplit)
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

	r.GET("/admin/logs/latest", adminHandler.GetLatestLogs)

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
