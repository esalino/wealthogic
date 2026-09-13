package tax

import (
	"testing"

	"github.com/eriksalino/wealthogic/api/internal/models"
)

// testContext builds an evaluation context from the shipped jurisdictions and
// rules, so these tests exercise the seed data users actually get rather than a
// hand-built fixture that could drift from it.
func testContext(accountTaxType string) Context {
	return Context{
		JurisdictionCodes: []string{models.JurisdictionUS, models.JurisdictionUSCA},
		AccountTaxType:    accountTaxType,
		Rules:             NewRuleSet(seedRules()),
		Tree:              NewTree(seedJurisdictions()),
	}
}

// outcome is one jurisdiction's answer, flattened for comparison.
type outcome struct {
	taxable   bool
	character string
	amount    float64
}

func outcomes(treatments []models.TaxTreatment) map[string]outcome {
	out := map[string]outcome{}
	for _, t := range treatments {
		amount := t.TaxableAmount
		if !t.Taxable {
			amount = t.ExcludedAmount
		}
		out[t.JurisdictionCode] = outcome{taxable: t.Taxable, character: t.Character, amount: amount}
	}
	return out
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name           string
		event          Event
		accountTaxType string
		want           map[string]outcome
	}{
		{
			// The U.S. rewards holding period; California does not. A single
			// event getting two different characters is the whole point of
			// evaluating per jurisdiction.
			name: "long-term equity gain",
			event: Event{
				IncomeType:    models.IncomeTypeCapitalGain,
				Amount:        1000,
				Term:          "long",
				AssetTaxClass: models.TaxClassEquity,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterLongTermCapital, amount: 1000},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: 1000},
			},
		},
		{
			name: "short-term equity gain",
			event: Event{
				IncomeType:    models.IncomeTypeCapitalGain,
				Amount:        500,
				Term:          "short",
				AssetTaxClass: models.TaxClassEquity,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterShortTermCapital, amount: 500},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: 500},
			},
		},
		{
			// A realized loss is carried with its sign; nothing about the
			// treatment changes.
			name: "capital loss keeps its sign",
			event: Event{
				IncomeType:    models.IncomeTypeCapitalGain,
				Amount:        -250,
				Term:          "long",
				AssetTaxClass: models.TaxClassEquity,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterLongTermCapital, amount: -250},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: -250},
			},
		},
		{
			// The case the old state_tax_exempt boolean existed to handle, now
			// falling out of the issuer relationship instead of a flag.
			name: "treasury interest is federally taxable and state exempt",
			event: Event{
				IncomeType:         models.IncomeTypeInterest,
				Amount:             300,
				AssetTaxClass:      models.TaxClassGovernmentBond,
				IssuerJurisdiction: models.JurisdictionUS,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterOrdinary, amount: 300},
				models.JurisdictionUSCA: {taxable: false, character: models.CharacterExempt, amount: 300},
			},
		},
		{
			// A Treasury fund reports dividends that are federal-obligation
			// interest underneath, so the exemption has to reach them too.
			name: "treasury fund dividend is state exempt",
			event: Event{
				IncomeType:         models.IncomeTypeDividend,
				Amount:             120,
				AssetTaxClass:      models.TaxClassGovernmentBond,
				IssuerJurisdiction: models.JurisdictionUS,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterOrdinary, amount: 120},
				models.JurisdictionUSCA: {taxable: false, character: models.CharacterExempt, amount: 120},
			},
		},
		{
			// Selling a Treasury is a capital gain, not interest - the
			// exemption covers the income, not the disposal.
			name: "treasury capital gain is taxable in both",
			event: Event{
				IncomeType:         models.IncomeTypeCapitalGain,
				Amount:             80,
				Term:               "short",
				AssetTaxClass:      models.TaxClassGovernmentBond,
				IssuerJurisdiction: models.JurisdictionUS,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterShortTermCapital, amount: 80},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: 80},
			},
		},
		{
			name: "in-state municipal interest is exempt in both",
			event: Event{
				IncomeType:         models.IncomeTypeInterest,
				Amount:             400,
				AssetTaxClass:      models.TaxClassMunicipalBond,
				IssuerJurisdiction: models.JurisdictionUSCA,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: false, character: models.CharacterExempt, amount: 400},
				models.JurisdictionUSCA: {taxable: false, character: models.CharacterExempt, amount: 400},
			},
		},
		{
			// The distinction the old boolean could not express at all: exempt
			// federally, taxable by the state that didn't issue it.
			name: "out-of-state municipal interest is federally exempt but state taxable",
			event: Event{
				IncomeType:         models.IncomeTypeInterest,
				Amount:             400,
				AssetTaxClass:      models.TaxClassMunicipalBond,
				IssuerJurisdiction: "US-NY",
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: false, character: models.CharacterExempt, amount: 400},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: 400},
			},
		},
		{
			name: "equity dividend is qualified-eligible federally, ordinary in CA",
			event: Event{
				IncomeType:    models.IncomeTypeDividend,
				Amount:        200,
				AssetTaxClass: models.TaxClassEquity,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterQualifiedEligible, amount: 200},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: 200},
			},
		},
		{
			// Brokers report money-market income as dividends, but it can never
			// qualify for the preferential rate.
			name: "money market dividend is ordinary everywhere",
			event: Event{
				IncomeType:    models.IncomeTypeDividend,
				Amount:        60,
				AssetTaxClass: models.TaxClassMoneyMarket,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterOrdinary, amount: 60},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: 60},
			},
		},
		{
			name: "IRA gain is deferred in every jurisdiction",
			event: Event{
				IncomeType:    models.IncomeTypeCapitalGain,
				Amount:        1000,
				Term:          "long",
				AssetTaxClass: models.TaxClassEquity,
			},
			accountTaxType: models.AccountTaxTypeIRA,
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: false, character: models.CharacterDeferred, amount: 1000},
				models.JurisdictionUSCA: {taxable: false, character: models.CharacterDeferred, amount: 1000},
			},
		},
		{
			name: "Roth dividend is exempt in every jurisdiction",
			event: Event{
				IncomeType:    models.IncomeTypeDividend,
				Amount:        200,
				AssetTaxClass: models.TaxClassEquity,
			},
			accountTaxType: models.AccountTaxTypeRoth,
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: false, character: models.CharacterExempt, amount: 200},
				models.JurisdictionUSCA: {taxable: false, character: models.CharacterExempt, amount: 200},
			},
		},
		{
			// An unclassified asset must still land somewhere taxable rather
			// than dropping out of the totals.
			name: "unclassified income falls through to ordinary",
			event: Event{
				IncomeType:    models.IncomeTypeInterest,
				Amount:        75,
				AssetTaxClass: models.TaxClassOther,
			},
			want: map[string]outcome{
				models.JurisdictionUS:   {taxable: true, character: models.CharacterOrdinary, amount: 75},
				models.JurisdictionUSCA: {taxable: true, character: models.CharacterOrdinary, amount: 75},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			accountTaxType := tc.accountTaxType
			if accountTaxType == "" {
				accountTaxType = models.AccountTaxTypePersonal
			}

			got := outcomes(Evaluate(tc.event, testContext(accountTaxType)))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d treatments, want %d", len(got), len(tc.want))
			}
			for code, want := range tc.want {
				if got[code] != want {
					t.Errorf("%s: got %+v, want %+v", code, got[code], want)
				}
			}
		})
	}
}

// A jurisdiction with no rules at all must still produce a treatment, or its
// income silently disappears from every total.
func TestEvaluateWithNoRulesForJurisdiction(t *testing.T) {
	ctx := Context{
		JurisdictionCodes: []string{"XX"},
		AccountTaxType:    models.AccountTaxTypePersonal,
		Rules:             NewRuleSet(nil),
		Tree:              NewTree(nil),
	}

	got := Evaluate(Event{IncomeType: models.IncomeTypeDividend, Amount: 100}, ctx)
	if len(got) != 1 {
		t.Fatalf("got %d treatments, want 1", len(got))
	}
	if !got[0].Taxable || got[0].TaxableAmount != 100 || got[0].Character != models.CharacterOrdinary {
		t.Errorf("unmatched event should be taxable ordinary income, got %+v", got[0])
	}
	if got[0].Reason != "no_matching_rule" {
		t.Errorf("got reason %q, want no_matching_rule", got[0].Reason)
	}
}

func TestIssuerScope(t *testing.T) {
	tree := NewTree(seedJurisdictions())

	tests := []struct {
		issuer, jurisdiction, want string
	}{
		{models.JurisdictionUS, models.JurisdictionUS, models.IssuerScopeSame},
		{models.JurisdictionUSCA, models.JurisdictionUSCA, models.IssuerScopeSame},
		// A state looking up at the federal government, and down at a state.
		{models.JurisdictionUS, models.JurisdictionUSCA, models.IssuerScopeAncestor},
		{models.JurisdictionUSCA, models.JurisdictionUS, models.IssuerScopeDescendant},
		// An unknown issuer is unrelated, not implicitly domestic.
		{"US-NY", models.JurisdictionUSCA, models.IssuerScopeForeign},
		{"", models.JurisdictionUS, models.IssuerScopeForeign},
	}

	for _, tc := range tests {
		if got := tree.IssuerScope(tc.issuer, tc.jurisdiction); got != tc.want {
			t.Errorf("IssuerScope(%q, %q) = %q, want %q", tc.issuer, tc.jurisdiction, got, tc.want)
		}
	}
}

// Rules must be consulted highest-priority first, which is what lets a carve-out
// beat the catch-all it sits above.
func TestRuleSetOrdersByPriority(t *testing.T) {
	rules := NewRuleSet(seedRules()).For(models.JurisdictionUSCA)
	if len(rules) == 0 {
		t.Fatal("no California rules seeded")
	}
	for i := 1; i < len(rules); i++ {
		if rules[i-1].Priority < rules[i].Priority {
			t.Fatalf("rules out of priority order at %d: %d then %d", i, rules[i-1].Priority, rules[i].Priority)
		}
	}
	if rules[len(rules)-1].Reason != "ordinary_income" {
		t.Errorf("last California rule should be the catch-all, got %q", rules[len(rules)-1].Reason)
	}
}

// Disposing of a Treasury realizes accreted discount, which is interest - so it
// must be evaluated as interest, not as the capital gain the import's "sell"
// makes it look like. Federally that's ordinary income; California can't tax it.
func TestTreasuryDisposalIsInterest(t *testing.T) {
	if got := models.DisposalIncomeType(models.TaxClassGovernmentBond); got != models.IncomeTypeInterest {
		t.Fatalf("government bond disposal = %q, want interest", got)
	}
	for _, class := range []string{models.TaxClassEquity, models.TaxClassMoneyMarket, models.TaxClassCorporateBond, models.TaxClassOther} {
		if got := models.DisposalIncomeType(class); got != models.IncomeTypeCapitalGain {
			t.Errorf("%s disposal = %q, want capital_gain", class, got)
		}
	}

	// The same disposal, evaluated the old way and the new way. The old
	// classification taxed it in California; the new one doesn't.
	ev := Event{
		Amount:             1021.15,
		AssetTaxClass:      models.TaxClassGovernmentBond,
		IssuerJurisdiction: models.JurisdictionUS,
	}
	ctx := testContext(models.AccountTaxTypePersonal)

	asCapital := ev
	asCapital.IncomeType = models.IncomeTypeCapitalGain
	asCapital.Term = "short"
	if got := outcomes(Evaluate(asCapital, ctx))[models.JurisdictionUSCA]; !got.taxable {
		t.Fatal("precondition: as a capital gain California would tax it")
	}

	asInterest := ev
	asInterest.IncomeType = models.IncomeTypeInterest
	got := outcomes(Evaluate(asInterest, ctx))

	want := map[string]outcome{
		models.JurisdictionUS:   {taxable: true, character: models.CharacterOrdinary, amount: 1021.15},
		models.JurisdictionUSCA: {taxable: false, character: models.CharacterExempt, amount: 1021.15},
	}
	for code, w := range want {
		if got[code] != w {
			t.Errorf("%s: got %+v, want %+v", code, got[code], w)
		}
	}
}
