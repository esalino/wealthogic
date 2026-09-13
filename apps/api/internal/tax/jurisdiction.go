// Package tax turns realized money events into per-jurisdiction tax treatments.
//
// The split of responsibilities is deliberate: an asset carries
// jurisdiction-neutral facts (what kind of income it produces, which government
// issued it), a jurisdiction carries rules, and the engine here combines the two
// into a persisted treatment per realized event. Supporting another country is
// then a matter of adding jurisdiction and rule rows, not changing this code.
package tax

import (
	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// Tree is a loaded snapshot of the jurisdiction hierarchy, used to resolve an
// asset's issuer against the jurisdiction whose rules are being applied.
type Tree struct {
	byCode map[string]models.TaxJurisdiction
}

// LoadTree reads every jurisdiction into memory. The set is tiny and changes
// only when a jurisdiction is added, so evaluation walks it rather than issuing
// a query per event.
func LoadTree(db *gorm.DB) (*Tree, error) {
	var rows []models.TaxJurisdiction
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	byCode := make(map[string]models.TaxJurisdiction, len(rows))
	for _, j := range rows {
		byCode[j.Code] = j
	}
	return &Tree{byCode: byCode}, nil
}

// NewTree builds a tree from an explicit set, for tests and seeding.
func NewTree(rows []models.TaxJurisdiction) *Tree {
	byCode := make(map[string]models.TaxJurisdiction, len(rows))
	for _, j := range rows {
		byCode[j.Code] = j
	}
	return &Tree{byCode: byCode}
}

// Get returns a jurisdiction by code.
func (t *Tree) Get(code string) (models.TaxJurisdiction, bool) {
	j, ok := t.byCode[code]
	return j, ok
}

// All returns every jurisdiction in the tree.
func (t *Tree) All() []models.TaxJurisdiction {
	out := make([]models.TaxJurisdiction, 0, len(t.byCode))
	for _, j := range t.byCode {
		out = append(out, j)
	}
	return out
}

// isAncestor reports whether ancestor sits above code in the tree.
func (t *Tree) isAncestor(ancestor, code string) bool {
	// Bounded by the tree's depth; the guard is only against a cyclic parent
	// link entered by hand in the database.
	for i := 0; i < len(t.byCode)+1; i++ {
		j, ok := t.byCode[code]
		if !ok || j.ParentCode == nil {
			return false
		}
		if *j.ParentCode == ancestor {
			return true
		}
		code = *j.ParentCode
	}
	return false
}

// IssuerScope classifies an asset's issuing jurisdiction relative to the
// jurisdiction whose rules are being applied. This relational view is what lets
// one rule row express "income from my own government's debt" or "from the
// government above me" without naming a country - the U.S. Treasury exemption
// in California and its equivalents elsewhere are the same rule.
//
// An asset with no recorded issuer is foreign: nothing is exempt by default.
func (t *Tree) IssuerScope(issuer, jurisdiction string) string {
	switch {
	case issuer == "":
		return models.IssuerScopeForeign
	case issuer == jurisdiction:
		return models.IssuerScopeSame
	case t.isAncestor(issuer, jurisdiction):
		return models.IssuerScopeAncestor
	case t.isAncestor(jurisdiction, issuer):
		return models.IssuerScopeDescendant
	}
	return models.IssuerScopeForeign
}
