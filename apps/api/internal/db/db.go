package db

import (
	"fmt"
	"log"
	"os"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/tax"
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

	if err := db.AutoMigrate(
		&models.User{}, &models.Account{}, &models.Holding{}, &models.Transaction{},
		&models.Gain{}, &models.Distribution{}, &models.Upload{}, &models.UploadTransaction{},
		&models.TaxJurisdiction{}, &models.TaxRule{}, &models.TaxProfile{}, &models.TaxTreatment{},
	); err != nil {
		return nil, err
	}

	// The shipped jurisdictions and rules must exist before any treatment can be
	// evaluated, including by the backfill below.
	if err := tax.Seed(db); err != nil {
		return nil, err
	}

	if err := migrateStateTaxExempt(db); err != nil {
		return nil, err
	}

	return db, nil
}

// migrateStateTaxExempt converts the retired per-holding state_tax_exempt flag
// into the jurisdiction-neutral tax attributes that replaced it, then drops the
// column. It's a no-op once the column is gone, so it's safe on every startup.
//
// The flag conflated an asset's nature with California's treatment of it, which
// is why the conversion is one-directional: "exempt" meant "this pays U.S.
// federal obligation interest", which is now a tax class plus an issuer. The
// reverse - an explicit "taxable" - carries no such fact, so it maps to nothing
// and the asset-type default takes over (already taxable for every class but
// Treasury).
func migrateStateTaxExempt(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&models.Holding{}, "state_tax_exempt") {
		return nil
	}

	usCode := models.JurisdictionUS
	if err := db.Model(&models.Holding{}).
		Where("state_tax_exempt IS TRUE").
		Updates(map[string]any{
			"tax_class_override":  models.TaxClassGovernmentBond,
			"issuer_jurisdiction": usCode,
		}).Error; err != nil {
		return err
	}

	// A Treasury explicitly marked state-taxable is the one case the flag can't
	// be read off: it says "not exempt" without saying what the asset is. Leave
	// it on the asset-type default and name it so it can be corrected by hand.
	var ambiguous []models.Holding
	if err := db.Where("state_tax_exempt IS FALSE AND asset_type = ?", models.AssetTypeTreasury).
		Find(&ambiguous).Error; err != nil {
		return err
	}
	for _, h := range ambiguous {
		log.Printf("tax migration: holding %s (%s) was marked state-taxable despite being a Treasury; "+
			"defaulted to tax class %s - set its tax class by hand if that's wrong",
			h.Symbol, h.ID, models.TaxClassGovernmentBond)
	}

	if err := db.Migrator().DropColumn(&models.Holding{}, "state_tax_exempt"); err != nil {
		return err
	}

	// Events realized before this migration have no treatments; build them from
	// the holdings' newly converted attributes.
	if err := db.Transaction(tax.RecomputeAll); err != nil {
		return err
	}

	log.Println("tax migration: converted state_tax_exempt to tax classes and rebuilt tax treatments")
	return nil
}
