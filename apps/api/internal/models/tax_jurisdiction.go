package models

import "time"

// Jurisdiction levels. A national jurisdiction is a country's top-level taxing
// authority (US federal); a regional one is a state/province/canton beneath it.
const (
	JurisdictionLevelNational = "national"
	JurisdictionLevelRegional = "regional"
)

// Jurisdiction codes we ship with. Regional codes are ISO 3166-2 style
// ("<country>-<region>") so the parent country is readable from the code.
const (
	JurisdictionUS   = "US"
	JurisdictionUSCA = "US-CA"
)

// TaxJurisdiction is one taxing authority. Jurisdictions form a tree via
// ParentCode (US-CA -> US), which is what lets a rule say "income from a bond
// issued by my parent government is exempt here" without naming a country -
// the shape of the U.S. Treasury state-tax exemption, and of its equivalents
// elsewhere. Seeded from code (see internal/tax.Seed), editable in the database.
type TaxJurisdiction struct {
	Code      string    `gorm:"primaryKey;size:16" json:"code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name  string `gorm:"not null" json:"name"`
	Level string `gorm:"not null" json:"level"` // national | regional

	// ParentCode is the jurisdiction this one sits inside, nil for a national
	// jurisdiction.
	ParentCode *string `gorm:"size:16;index" json:"parent_code"`

	CurrencyCode string `gorm:"size:3" json:"currency_code"`
} // @name TaxJurisdiction
