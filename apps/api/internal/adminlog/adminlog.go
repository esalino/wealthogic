// Package adminlog records runs of the maintenance utilities on the Admin page.
//
// It exists so the Admin page can answer "when did I last run this?" across
// sessions. The utilities themselves are unaffected by it: recording is
// best-effort, and a bookkeeping row that fails to save must never fail a sweep
// that already did its work.
package adminlog

import (
	"log"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// Utility names, as stored in the log and sent to the client. Stable strings
// rather than an enum: the client keys its per-tool headers off them.
const (
	UtilityRefreshPrices       = "refresh_holding_prices"
	UtilityBackfillProfiles    = "backfill_holding_profiles"
	UtilityRecalculateHoldings = "recalculate_holdings"
)

// Run is an in-flight utility run. Start one, do the work, then Finish it.
type Run struct {
	utility   string
	startedAt time.Time
}

// Start marks the beginning of a run.
func Start(utility string) Run {
	return Run{utility: utility, startedAt: time.Now().UTC()}
}

// Finish writes the run to the log, with errs as the problems it hit (nil or
// empty for a clean run).
//
// It returns nothing on purpose. The caller has already finished its real work
// by this point, and there is no sensible way for it to react to the log row
// failing to save - so a failure is logged here and the sweep still succeeds.
func (r Run) Finish(db *gorm.DB, errs []string) {
	finishedAt := time.Now().UTC()

	entry := models.AdminLog{
		Utility:    r.utility,
		StartedAt:  r.startedAt,
		FinishedAt: finishedAt,
		DurationMS: finishedAt.Sub(r.startedAt).Milliseconds(),
	}
	if joined := strings.TrimSpace(strings.Join(errs, "\n")); joined != "" {
		entry.Errors = &joined
	}

	if err := db.Create(&entry).Error; err != nil {
		log.Printf("admin log: failed to record %s run: %v", r.utility, err)
	}
}

// LatestPerUtility returns the most recent run of each utility, newest first.
//
// One row per utility is what the Admin page needs - a last-run line per tool -
// so the whole page costs a single query rather than one per card.
func LatestPerUtility(db *gorm.DB) ([]models.AdminLog, error) {
	// Non-nil so an empty log serializes as [] rather than null - a client
	// iterating the result shouldn't have to special-case "no runs yet".
	logs := []models.AdminLog{}
	err := db.Raw(`
		SELECT DISTINCT ON (utility) *
		FROM admin_logs
		ORDER BY utility, started_at DESC
	`).Scan(&logs).Error
	return logs, err
}
