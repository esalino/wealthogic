package models

import (
	"time"

	"github.com/google/uuid"
)

// Reasons recorded on a treatment beyond the rule's own. ReasonShelteredAccount
// is set by the short-circuit for tax-advantaged accounts, which applies before
// any jurisdiction rule.
const (
	ReasonShelteredAccount = "tax_advantaged_account"
)

// TaxTreatment is how one jurisdiction treats one realized event: each
// RealizedEvent gets one treatment per jurisdiction the taxpayer is subject to,
// written when the event is realized.
//
// Persisting the outcome per jurisdiction (rather than deriving it on read) is
// what lets the Tax Center total a year's income per jurisdiction and character
// in SQL, and what lets each figure point at the rule that produced it.
//
// Rebuilt whenever its event is, so no soft delete.
type TaxTreatment struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// RealizedEventID is a real foreign key: one ledger means this can be a
	// plain association rather than a polymorphic (type, id) pair, so treatments
	// preload with their events instead of being stitched together by hand.
	RealizedEventID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tax_treatment_event,priority:1" json:"realized_event_id"`

	JurisdictionCode string `gorm:"size:16;not null;uniqueIndex:idx_tax_treatment_event,priority:2;index:idx_tax_treatment_year,priority:2" json:"jurisdiction_code"`

	// TaxYear is denormalized from the event so a year's per-jurisdiction
	// summary stays a single grouped query with no join. It's rewritten with the
	// treatment, so it can't drift from the event it came from.
	TaxYear int `gorm:"not null;index:idx_tax_treatment_year,priority:1" json:"tax_year"`

	Taxable bool `gorm:"not null" json:"taxable"`

	// TaxableAmount and ExcludedAmount split the event's amount by this
	// jurisdiction's treatment; exactly one is non-zero. Keeping the excluded
	// side rather than dropping it is what lets the UI show what was left out
	// ("Excluded - U.S. Treasury interest") instead of a silently smaller total.
	TaxableAmount  float64 `gorm:"not null;default:0" json:"taxable_amount"`
	ExcludedAmount float64 `gorm:"not null;default:0" json:"excluded_amount"`

	// Stored as tax_character: "character" is a type name in Postgres and can't
	// be used unquoted as a column.
	Character string `gorm:"column:tax_character;size:32;not null" json:"character"`

	// RuleID is the rule that decided this, nil when no rule was consulted (a
	// sheltered account) or none matched. Reason is its machine-readable label.
	RuleID *uuid.UUID `gorm:"type:uuid" json:"rule_id"`
	Reason string     `gorm:"size:64" json:"reason"`
} // @name TaxTreatment
