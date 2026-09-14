package tax

import (
	"sort"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Event is a realized money event reduced to the facts the rules care about,
// so the engine depends on nothing but those facts.
type Event struct {
	RealizedEventID uuid.UUID

	IncomeType string  // models.IncomeType*
	Amount     float64 // the realized gain/loss or the income paid
	Term       string  // "short" | "long"; empty outside capital gains

	AssetTaxClass      string // models.TaxClass*
	IssuerJurisdiction string // "" when the asset has no issuing government

	TaxYear int
}

// Context is everything an evaluation needs besides the event: who the taxpayer
// is subject to, what shelter the account provides, and the rules in force.
type Context struct {
	// JurisdictionCodes is the ordered list the taxpayer is subject to, national
	// first. One treatment is produced per code.
	JurisdictionCodes []string

	// AccountTaxType is the account's shelter (models.AccountTaxType*), checked
	// before any jurisdiction rule.
	AccountTaxType string

	Rules *RuleSet
	Tree  *Tree
}

// RuleSet is the rules in force, indexed by jurisdiction and pre-sorted so
// evaluation is a linear scan of the candidates for one jurisdiction.
type RuleSet struct {
	byJurisdiction map[string][]models.TaxRule
}

// NewRuleSet indexes rules by jurisdiction, highest priority first. Ties keep
// their input order, so seed order is the documented tiebreak.
func NewRuleSet(rules []models.TaxRule) *RuleSet {
	byJurisdiction := map[string][]models.TaxRule{}
	for _, r := range rules {
		byJurisdiction[r.JurisdictionCode] = append(byJurisdiction[r.JurisdictionCode], r)
	}
	for code := range byJurisdiction {
		rs := byJurisdiction[code]
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].Priority > rs[j].Priority })
	}
	return &RuleSet{byJurisdiction: byJurisdiction}
}

// LoadRuleSet reads every rule from the database.
func LoadRuleSet(db *gorm.DB) (*RuleSet, error) {
	var rules []models.TaxRule
	if err := db.Find(&rules).Error; err != nil {
		return nil, err
	}
	return NewRuleSet(rules), nil
}

// For returns one jurisdiction's rules, highest priority first.
func (s *RuleSet) For(jurisdiction string) []models.TaxRule {
	return s.byJurisdiction[jurisdiction]
}

// isSheltered reports whether an account type defers or eliminates tax on the
// activity inside it, and with what character. A sheltered account settles the
// question everywhere at once, so it's checked before any jurisdiction rule.
func isSheltered(accountTaxType string) (character string, sheltered bool) {
	switch accountTaxType {
	case models.AccountTaxTypeIRA:
		// Taxed on withdrawal instead, not here.
		return models.CharacterDeferred, true
	case models.AccountTaxTypeRoth:
		return models.CharacterExempt, true
	}
	return "", false
}

// Evaluate produces one treatment per jurisdiction the taxpayer is subject to.
//
// For each jurisdiction the highest-priority matching rule wins; when nothing
// matches, the event is treated as ordinary taxable income, which is the safe
// default for a jurisdiction whose rules are incomplete - it over-reports rather
// than silently dropping income.
func Evaluate(ev Event, ctx Context) []models.TaxTreatment {
	shelterCharacter, sheltered := isSheltered(ctx.AccountTaxType)

	treatments := make([]models.TaxTreatment, 0, len(ctx.JurisdictionCodes))
	for _, code := range ctx.JurisdictionCodes {
		t := models.TaxTreatment{
			RealizedEventID:  ev.RealizedEventID,
			JurisdictionCode: code,
			TaxYear:          ev.TaxYear,
		}

		if sheltered {
			t.Taxable = false
			t.ExcludedAmount = ev.Amount
			t.Character = shelterCharacter
			t.Reason = models.ReasonShelteredAccount
			treatments = append(treatments, t)
			continue
		}

		scope := ctx.Tree.IssuerScope(ev.IssuerJurisdiction, code)
		rule, matched := matchRule(ctx.Rules.For(code), ev, scope)

		switch {
		case !matched:
			t.Taxable = true
			t.TaxableAmount = ev.Amount
			t.Character = models.CharacterOrdinary
			t.Reason = "no_matching_rule"
		case rule.Treatment == models.TreatmentExempt:
			t.Taxable = false
			t.ExcludedAmount = ev.Amount
			t.Character = models.CharacterExempt
			t.RuleID = &rule.ID
			t.Reason = rule.Reason
		default:
			t.Taxable = true
			t.TaxableAmount = ev.Amount
			t.Character = rule.Character
			t.RuleID = &rule.ID
			t.Reason = rule.Reason
		}

		treatments = append(treatments, t)
	}
	return treatments
}

// matchRule returns the first (highest-priority) rule matching the event.
func matchRule(rules []models.TaxRule, ev Event, issuerScope string) (models.TaxRule, bool) {
	for _, r := range rules {
		if r.Matches(ev.IncomeType, ev.AssetTaxClass, ev.Term, issuerScope) {
			return r, true
		}
	}
	return models.TaxRule{}, false
}
