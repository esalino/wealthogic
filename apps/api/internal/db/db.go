package db

import (
	"fmt"
	"log"
	"os"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/tax"
	"github.com/google/uuid"
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

	if err := reclassifyDisposals(db); err != nil {
		return nil, err
	}

	return db, nil
}

// reclassifyDisposals corrects Gain rows whose category disagrees with what
// their holding's tax class says the disposal realized - the case being a
// Treasury redemption, imported as a sell and so recorded as a capital gain when
// the money is really accreted discount, i.e. interest.
//
// The check is a comparison rather than a one-shot migration because the
// category is derived: it repairs old rows once and stays a cheap no-op after,
// while also catching drift if a holding's class is corrected outside the normal
// recompute path.
func reclassifyDisposals(db *gorm.DB) error {
	var holdings []models.Holding
	if err := db.Find(&holdings).Error; err != nil {
		return err
	}

	// Group holdings by the category their disposals should carry, so the check
	// is one query per category rather than one per holding.
	wantByCategory := map[string][]uuid.UUID{}
	for _, h := range holdings {
		want := models.DisposalIncomeType(h.ResolveTaxClass())
		wantByCategory[want] = append(wantByCategory[want], h.ID)
	}

	var mismatched int64
	for want, ids := range wantByCategory {
		var count int64
		if err := db.Model(&models.Gain{}).
			Where("holding_id IN ? AND category <> ?", ids, want).
			Count(&count).Error; err != nil {
			return err
		}
		mismatched += count
	}
	if mismatched == 0 {
		return nil
	}

	// RecomputeAll reclassifies each gain and rewrites its treatments, so the
	// correction and the tax consequences of it land together.
	if err := db.Transaction(tax.RecomputeAll); err != nil {
		return err
	}

	log.Printf("tax migration: reclassified %d realized disposal(s) to match their asset's tax class "+
		"(e.g. Treasury redemptions from capital gain to interest) and rebuilt their tax treatments", mismatched)
	return nil
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
