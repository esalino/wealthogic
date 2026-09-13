package models

import (
	"time"

	"github.com/google/uuid"
)

// The ledgers a tax treatment can attach to.
const (
	TaxSourceGain         = "gain"
	TaxSourceDistribution = "distribution"
)

// Reasons recorded on a treatment beyond the rule's own. ReasonShelteredAccount
// is set by the short-circuit for tax-advantaged accounts, which applies before
// any jurisdiction rule runs.
const (
	ReasonShelteredAccount = "tax_advantaged_account"
)

// TaxTreatment is how one jurisdiction treats one realized taxable event: a
// Gain row or a Distribution row gets one treatment per jurisdiction the
// taxpayer is subject to, written when the event is realized.
//
// Persisting the outcome per jurisdiction (rather than deriving it on read) is
// what lets the Tax Center total a year's income per jurisdiction and character
// in SQL, and what lets each figure point at the rule that produced it.
//
// Like Gain and Distribution this is a derived ledger, rebuilt when its source
// or the rules change, so it carries no soft delete.
type TaxTreatment struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// The realized event this treatment applies to. SourceType distinguishes the
	// two ledgers (capital gains vs. income) rather than splitting this into two
	// near-identical tables.
	SourceType string    `gorm:"size:16;not null;uniqueIndex:idx_tax_treatment_source,priority:1" json:"source_type"`
	SourceID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tax_treatment_source,priority:2" json:"source_id"`

	JurisdictionCode string `gorm:"size:16;not null;uniqueIndex:idx_tax_treatment_source,priority:3;index:idx_tax_treatment_year,priority:2" json:"jurisdiction_code"`

	// TaxYear and AccountID are denormalized from the source row so a year's
	// per-jurisdiction summary is a single grouped query with no joins.
	TaxYear   int        `gorm:"not null;index:idx_tax_treatment_year,priority:1" json:"tax_year"`
	AccountID uuid.UUID  `gorm:"type:uuid;index" json:"account_id"`
	HoldingID *uuid.UUID `gorm:"type:uuid;index" json:"holding_id"`

	Taxable bool `gorm:"not null" json:"taxable"`

	// TaxableAmount and ExcludedAmount split the source amount by this
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
