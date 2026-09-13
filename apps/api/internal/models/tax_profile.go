package models

import (
	"time"

	"github.com/google/uuid"
)

// Account tax types. A sheltered account defers or eliminates tax on the
// activity inside it regardless of jurisdiction, so it's checked before any
// jurisdiction rule. The names are U.S.-specific today but the concept isn't
// (ISA, TFSA, and friends behave the same way).
const (
	AccountTaxTypePersonal = "Personal"
	AccountTaxTypeIRA      = "IRA"
	AccountTaxTypeRoth     = "Roth"
)

// TaxProfile is where a person is taxed: a country and, where the country taxes
// regionally, a region. It resolves to the ordered list of jurisdictions whose
// rules apply to that person's realized income.
//
// Residency belongs to the person rather than the account, so this hangs off
// User; an account's jurisdictions come from its owners.
type TaxProfile struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`

	// CountryCode is the national jurisdiction's code; RegionCode the regional
	// one, nil where the country has no regional income tax or the person isn't
	// subject to one.
	CountryCode string  `gorm:"size:16;not null" json:"country_code"`
	RegionCode  *string `gorm:"size:16" json:"region_code"`

	// EffectiveFrom is when this residency started. Only the current profile is
	// consulted today; storing the date means a residency history can be added
	// later without a migration.
	EffectiveFrom time.Time `gorm:"type:date" json:"effective_from"`
} // @name TaxProfile

// JurisdictionCodes is the ordered list of jurisdictions this profile is
// subject to, national first.
func (p TaxProfile) JurisdictionCodes() []string {
	codes := []string{p.CountryCode}
	if p.RegionCode != nil && *p.RegionCode != "" {
		codes = append(codes, *p.RegionCode)
	}
	return codes
}
