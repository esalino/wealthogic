package tax

import (
	"sort"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// Priority bands for seeded rules. A rule's priority is what orders the match,
// so keeping the bands named makes the intent of a row readable: carve-outs beat
// specific rules, which beat the catch-all.
const (
	priorityFallback = 10 // catch-all for an income type
	prioritySpecific = 20 // a class-specific character, e.g. equity dividends
	priorityCarveOut = 40 // an exemption that must win over everything above
)

// usStates are the regional jurisdictions beneath the U.S. All of them are
// seeded even though only California carries rules today, because a
// jurisdiction is an issuer identity as well as a rule holder: a New York
// municipal bond needs US-NY to exist for the tree to place it, or its interest
// can't be recognized as a state obligation at all. Adding rules for another
// state later is then purely additive.
var usStates = map[string]string{
	"US-AL": "Alabama", "US-AK": "Alaska", "US-AZ": "Arizona", "US-AR": "Arkansas",
	"US-CA": "California", "US-CO": "Colorado", "US-CT": "Connecticut", "US-DE": "Delaware",
	"US-DC": "District of Columbia", "US-FL": "Florida", "US-GA": "Georgia", "US-HI": "Hawaii",
	"US-ID": "Idaho", "US-IL": "Illinois", "US-IN": "Indiana", "US-IA": "Iowa",
	"US-KS": "Kansas", "US-KY": "Kentucky", "US-LA": "Louisiana", "US-ME": "Maine",
	"US-MD": "Maryland", "US-MA": "Massachusetts", "US-MI": "Michigan", "US-MN": "Minnesota",
	"US-MS": "Mississippi", "US-MO": "Missouri", "US-MT": "Montana", "US-NE": "Nebraska",
	"US-NV": "Nevada", "US-NH": "New Hampshire", "US-NJ": "New Jersey", "US-NM": "New Mexico",
	"US-NY": "New York", "US-NC": "North Carolina", "US-ND": "North Dakota", "US-OH": "Ohio",
	"US-OK": "Oklahoma", "US-OR": "Oregon", "US-PA": "Pennsylvania", "US-RI": "Rhode Island",
	"US-SC": "South Carolina", "US-SD": "South Dakota", "US-TN": "Tennessee", "US-TX": "Texas",
	"US-UT": "Utah", "US-VT": "Vermont", "US-VA": "Virginia", "US-WA": "Washington",
	"US-WV": "West Virginia", "US-WI": "Wisconsin", "US-WY": "Wyoming",
}

// seedJurisdictions is the jurisdiction tree shipped with the app. The parent
// link from each state up to US is what makes the Treasury exemption
// expressible as a relationship ("issued by the government above me") rather
// than a hardcoded pair.
func seedJurisdictions() []models.TaxJurisdiction {
	us := models.JurisdictionUS
	out := []models.TaxJurisdiction{{
		Code:         models.JurisdictionUS,
		Name:         "United States (Federal)",
		Level:        models.JurisdictionLevelNational,
		CurrencyCode: "USD",
	}}

	codes := make([]string, 0, len(usStates))
	for code := range usStates {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		out = append(out, models.TaxJurisdiction{
			Code:         code,
			Name:         usStates[code],
			Level:        models.JurisdictionLevelRegional,
			ParentCode:   &us,
			CurrencyCode: "USD",
		})
	}
	return out
}

// seedRules is the tax treatment shipped for the jurisdictions above.
//
// Every jurisdiction ends with catch-all rows so an event never falls through
// to the engine's "no matching rule" path in normal operation. Carve-outs sit
// above them at priorityCarveOut.
func seedRules() []models.TaxRule {
	return append(federalRules(), californiaRules()...)
}

// federalRules: the U.S. splits capital gains by holding period, gives equity
// dividends a character that may qualify for a preferential rate, and exempts
// interest on debt issued by its own states and their subdivisions.
func federalRules() []models.TaxRule {
	j := models.JurisdictionUS
	return []models.TaxRule{
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeInterest,
			AssetTaxClass:    models.TaxClassMunicipalBond,
			IssuerScope:      models.IssuerScopeDescendant,
			Treatment:        models.TreatmentExempt,
			Character:        models.CharacterExempt,
			Priority:         priorityCarveOut,
			Reason:           "municipal_interest_federal_exempt",
			Note:             "Interest on debt issued by a state or its subdivisions is excluded from federal gross income.",
		},
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeCapitalGain,
			Term:             "long",
			Treatment:        models.TreatmentTaxable,
			Character:        models.CharacterLongTermCapital,
			Priority:         prioritySpecific,
			Reason:           "long_term_capital_gain",
			Note:             "Held more than one year; eligible for the long-term capital gains rates.",
		},
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeCapitalGain,
			Term:             "short",
			Treatment:        models.TreatmentTaxable,
			Character:        models.CharacterShortTermCapital,
			Priority:         prioritySpecific,
			Reason:           "short_term_capital_gain",
			Note:             "Held one year or less; taxed at ordinary rates.",
		},
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeDividend,
			AssetTaxClass:    models.TaxClassEquity,
			Treatment:        models.TreatmentTaxable,
			Character:        models.CharacterQualifiedEligible,
			Priority:         prioritySpecific,
			Reason:           "equity_dividend",
			Note:             "Equity dividends may be qualified; the holding-period test is not modeled yet.",
		},
		// Money-market and government-bond distributions are interest in
		// substance however the broker labels them, so they never carry the
		// qualified-eligible character.
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeDividend,
			AssetTaxClass:    models.TaxClassMoneyMarket,
			Treatment:        models.TreatmentTaxable,
			Character:        models.CharacterOrdinary,
			Priority:         prioritySpecific,
			Reason:           "money_market_income",
			Note:             "Money-market distributions are ordinary income despite being reported as dividends.",
		},
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeDividend,
			AssetTaxClass:    models.TaxClassGovernmentBond,
			Treatment:        models.TreatmentTaxable,
			Character:        models.CharacterOrdinary,
			Priority:         prioritySpecific,
			Reason:           "government_bond_income",
			Note:             "Treasury income is fully taxable at the federal level.",
		},
		{
			JurisdictionCode: j,
			Treatment:        models.TreatmentTaxable,
			Character:        models.CharacterOrdinary,
			Priority:         priorityFallback,
			Reason:           "ordinary_income",
			Note:             "Catch-all: taxable as ordinary income.",
		},
	}
}

// californiaRules: California makes no long/short capital gains distinction and
// has no preferential dividend rate - everything is ordinary income - but it
// cannot tax interest on federal debt, and exempts its own municipal debt.
func californiaRules() []models.TaxRule {
	j := models.JurisdictionUSCA
	return []models.TaxRule{
		// The exemption applies however the payer labels the income, so it's
		// seeded for both interest and dividends: a Treasury money-market fund
		// reports dividends that are federal-obligation interest underneath.
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeInterest,
			AssetTaxClass:    models.TaxClassGovernmentBond,
			IssuerScope:      models.IssuerScopeAncestor,
			Treatment:        models.TreatmentExempt,
			Character:        models.CharacterExempt,
			Priority:         priorityCarveOut,
			Reason:           "federal_obligation_state_exempt",
			Note:             "A state may not tax interest on the debt of the government above it.",
		},
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeDividend,
			AssetTaxClass:    models.TaxClassGovernmentBond,
			IssuerScope:      models.IssuerScopeAncestor,
			Treatment:        models.TreatmentExempt,
			Character:        models.CharacterExempt,
			Priority:         priorityCarveOut,
			Reason:           "federal_obligation_state_exempt",
			Note:             "Treasury fund distributions are federal-obligation interest and exempt on the same basis.",
		},
		{
			JurisdictionCode: j,
			IncomeType:       models.IncomeTypeInterest,
			AssetTaxClass:    models.TaxClassMunicipalBond,
			IssuerScope:      models.IssuerScopeSame,
			Treatment:        models.TreatmentExempt,
			Character:        models.CharacterExempt,
			Priority:         priorityCarveOut,
			Reason:           "in_state_municipal_exempt",
			Note:             "In-state municipal interest is exempt; out-of-state municipal interest is not.",
		},
		// No long/short split and no preferential dividend rate: one ordinary
		// catch-all covers capital gains, dividends, and interest alike.
		{
			JurisdictionCode: j,
			Treatment:        models.TreatmentTaxable,
			Character:        models.CharacterOrdinary,
			Priority:         priorityFallback,
			Reason:           "ordinary_income",
			Note:             "California taxes capital gains and dividends as ordinary income.",
		},
	}
}

// Seed inserts the shipped jurisdictions and rules. It is idempotent and
// additive: a row already present (matched on its natural key) is left alone, so
// rules edited in the database survive a restart and a newly shipped rule still
// arrives. It never deletes, so removing a shipped rule is a deliberate act.
func Seed(db *gorm.DB) error {
	for _, j := range seedJurisdictions() {
		var count int64
		if err := db.Model(&models.TaxJurisdiction{}).Where("code = ?", j.Code).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := db.Create(&j).Error; err != nil {
			return err
		}
	}

	for _, r := range seedRules() {
		// Natural key: a jurisdiction's treatment of one (income type, asset
		// class, term, issuer scope) combination is unique by construction.
		var count int64
		if err := db.Model(&models.TaxRule{}).
			Where("jurisdiction_code = ? AND income_type = ? AND asset_tax_class = ? AND term = ? AND issuer_scope = ?",
				r.JurisdictionCode, r.IncomeType, r.AssetTaxClass, r.Term, r.IssuerScope).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if err := db.Create(&r).Error; err != nil {
			return err
		}
	}
	return nil
}
