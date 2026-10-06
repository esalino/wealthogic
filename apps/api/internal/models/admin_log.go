package models

import (
	"time"

	"github.com/google/uuid"
)

// AdminLog records one run of a maintenance utility, so the Admin page can show
// when each tool was last run without inferring it from the data the run
// touched - a price stamp tells you when a holding was priced, not whether the
// sweep ran and found nothing to do.
//
// Append-only and disposable: these rows describe operations rather than the
// portfolio, nothing but the Admin page reads them, and deleting them loses
// nothing but the history. Hence no soft delete.
type AdminLog struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`

	// Utility names the tool that ran - see the Utility* constants in
	// internal/adminlog. Indexed with StartedAt so "the latest run of each
	// utility" is an index scan.
	Utility string `gorm:"not null;index:idx_admin_log_utility,priority:1" json:"utility"`

	StartedAt  time.Time `gorm:"not null;index:idx_admin_log_utility,priority:2" json:"started_at"`
	FinishedAt time.Time `gorm:"not null"                                       json:"finished_at"`

	// DurationMS is how long the run took, stored so reading "took 4s" needs no
	// arithmetic over the two timestamps.
	DurationMS int64 `gorm:"not null" json:"duration_ms"`

	// Errors is what went wrong, one per line, or nil for a clean run.
	//
	// It is independent of the run having completed: a sweep can finish having
	// failed on some of its items - a symbol the quote provider won't price -
	// and that is a successful run with errors in it, not a failure.
	Errors *string `gorm:"type:text" json:"errors"`
} // @name AdminLog
