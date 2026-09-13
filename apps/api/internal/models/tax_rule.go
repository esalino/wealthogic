package models

import (
	"time"

	"github.com/google/uuid"
)

// Income types a taxable event can have. These describe how the money arrived,
// not how it's taxed - a jurisdiction's rules decide that.
const (
	IncomeTypeCapitalGain = "capital_gain"
	IncomeTypeDividend    = "dividend"
	IncomeTypeInterest    = "interest"
)

// Asset tax classes: what kind of income an asset produces, as a property of
// the asset itself rather than of any one jurisdiction's treatment of it. This
// is the jurisdiction-neutral replacement for the old state_tax_exempt boolean -
// "this fund holds U.S. Treasuries" is true everywhere, while "exempt from
// state tax" is only true for some taxpayers.
const (
	TaxClassEquity         = "equity"
	TaxClassGovernmentBond = "government_bond" // sovereign/national debt, e.g. U.S. Treasuries
	TaxClassMunicipalBond  = "municipal_bond"  // sub-national government debt
	TaxClassCorporateBond  = "corporate_bond"
	TaxClassMoneyMarket    = "money_market"
	TaxClassOther          = "other"
)

// Issuer scopes, resolved at evaluation time by comparing the asset's issuing
// jurisdiction to the jurisdiction whose rules are being applied. Expressing
// the matcher relationally (rather than as a literal code) is what keeps a rule
// portable: "exempt when my parent government issued it" is the U.S. Treasury
// state exemption in California and the equivalent rule elsewhere.
const (
	IssuerScopeSame       = "same"       // issued by this jurisdiction
	IssuerScopeAncestor   = "ancestor"   // issued by a jurisdiction above this one (US, viewed from US-CA)
	IssuerScopeDescendant = "descendant" // issued by a jurisdiction below this one (US-CA, viewed from US)
	IssuerScopeForeign    = "foreign"    // unrelated jurisdiction, or none recorded
)

// Rule outcomes.
const (
	TreatmentTaxable = "taxable"
	TreatmentExempt  = "exempt"
)

// Tax characters: the bucket taxable income lands in within a jurisdiction.
// Jurisdictions differ here - the U.S. splits capital gains by holding period
// while California folds them into ordinary income - which is precisely what
// the old single "taxable amount" model could not represent.
const (
	CharacterLongTermCapital   = "long_term_capital"
	CharacterShortTermCapital  = "short_term_capital"
	CharacterQualifiedEligible = "qualified_eligible" // dividends that may qualify for a preferential rate
	CharacterOrdinary          = "ordinary"
	CharacterExempt            = "exempt"
	CharacterDeferred          = "deferred" // taxed later, on distribution from a sheltered account
)

// TaxRule is one jurisdiction's treatment of a class of taxable event. The
// match columns narrow what the rule applies to; an empty match column means
// "any". Evaluation takes the highest-priority matching rule for a jurisdiction
// (ties broken by insertion order), so a catch-all sits at a low priority and
// specific carve-outs sit above it.
//
// Rules live in the database and are seeded from code (see internal/tax.Seed),
// so supporting a new jurisdiction is a matter of adding rows rather than
// changing the engine.
type TaxRule struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	JurisdictionCode string `gorm:"size:16;not null;index" json:"jurisdiction_code"`

	// Match columns. Empty string = matches anything.
	IncomeType    string `gorm:"size:32;not null;default:''" json:"income_type"`
	AssetTaxClass string `gorm:"size:32;not null;default:''" json:"asset_tax_class"`
	Term          string `gorm:"size:16;not null;default:''" json:"term"`         // short | long, capital gains only
	IssuerScope   string `gorm:"size:16;not null;default:''" json:"issuer_scope"` // see IssuerScope* above

	// Outcome.
	Treatment string `gorm:"size:16;not null" json:"treatment"`
	// Stored as tax_character: "character" is a type name in Postgres and can't
	// be used unquoted as a column.
	Character string `gorm:"column:tax_character;size:32;not null" json:"character"`

	// Priority orders the match; highest wins. Reason is the machine-readable
	// label carried onto the resulting treatment so the UI can explain an
	// exclusion ("Excluded - U.S. Treasury interest").
	Priority int    `gorm:"not null;default:0" json:"priority"`
	Reason   string `gorm:"size:64" json:"reason"`
	Note     string `json:"note"`
} // @name TaxRule

// Matches reports whether this rule applies to an event with the given
// attributes. An empty match column is a wildcard.
func (r TaxRule) Matches(incomeType, assetTaxClass, term, issuerScope string) bool {
	return matchField(r.IncomeType, incomeType) &&
		matchField(r.AssetTaxClass, assetTaxClass) &&
		matchField(r.Term, term) &&
		matchField(r.IssuerScope, issuerScope)
}

// matchField compares one match column against an event's value: an empty
// column matches anything.
func matchField(ruleValue, eventValue string) bool {
	return ruleValue == "" || ruleValue == eventValue
}
