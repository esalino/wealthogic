package db

import (
	"fmt"
	"os"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Connect() (*gorm.DB, error) {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	// TimeZone=UTC keeps the session in UTC so date-only columns (transaction
	// and tax-lot dates, parsed as UTC midnight) don't shift a day when Postgres
	// casts the timestamp to a date in a non-UTC server timezone.
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		host,
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
		os.Getenv("POSTGRES_PORT"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&models.User{}, &models.Account{}, &models.Holding{}, &models.Transaction{}, &models.Gain{}, &models.Upload{}, &models.UploadTransaction{}); err != nil {
		return nil, err
	}

	return db, nil
}
