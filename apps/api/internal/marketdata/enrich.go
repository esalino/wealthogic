package marketdata

import (
	"context"
	"log"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// Enricher fills in a holding's reference data from a provider.
//
// Every method is safe to call with a nil Enricher, which is what a build with
// no API key configured produces - enrichment then does nothing and the caller
// needs no special case.
type Enricher struct {
	provider Provider
}

// NewEnricher wraps a provider. A nil provider yields a nil Enricher, so
// "no key configured" and "no enricher" are the same thing to callers.
func NewEnricher(provider Provider) *Enricher {
	if provider == nil {
		return nil
	}
	return &Enricher{provider: provider}
}

// Enabled reports whether enrichment will actually do anything.
func (e *Enricher) Enabled() bool { return e != nil && e.provider != nil }

// Enrich looks up one holding and saves whatever the provider knows.
//
// It reports an error only so callers can log it; none should fail their own
// work over it. Reference data is descriptive, and a holding without a sector
// is still a perfectly good holding.
func (e *Enricher) Enrich(ctx context.Context, db *gorm.DB, holding *models.Holding) error {
	if !e.Enabled() || !holding.WantsProfile() {
		return nil
	}

	profile, found, err := e.provider.FetchProfile(ctx, holding.Symbol)
	if err != nil {
		return err
	}

	// Stamp the attempt either way. A symbol the provider doesn't cover is a
	// settled answer, not something to ask again on the next import.
	now := time.Now().UTC()
	updates := map[string]any{"profile_fetched_at": now}
	holding.ProfileFetchedAt = &now

	if found {
		holding.CompanyName = profile.CompanyName
		holding.CompanyDescription = profile.Description
		holding.Sector = profile.Sector
		holding.Industry = profile.Industry
		holding.Exchange = profile.Exchange
		holding.Country = profile.Country
		holding.Website = profile.Website
		holding.LogoURL = profile.LogoURL

		updates["company_name"] = profile.CompanyName
		updates["company_description"] = profile.Description
		updates["sector"] = profile.Sector
		updates["industry"] = profile.Industry
		updates["exchange"] = profile.Exchange
		updates["country"] = profile.Country
		updates["website"] = profile.Website
		updates["logo_url"] = profile.LogoURL
	}

	return db.Model(holding).Updates(updates).Error
}

// EnrichQuietly runs Enrich and logs any failure instead of returning it, for
// the paths where a holding has just been created and the reference lookup is
// strictly a bonus.
func (e *Enricher) EnrichQuietly(ctx context.Context, db *gorm.DB, holding *models.Holding) {
	if err := e.Enrich(ctx, db, holding); err != nil {
		log.Printf("market data: profile lookup for %s failed: %v", holding.Symbol, err)
	}
}

// BackfillResult reports what a backfill pass did.
type BackfillResult struct {
	Considered int `json:"considered"`
	Enriched   int `json:"enriched"`
	NotFound   int `json:"not_found"`
	Failed     int `json:"failed"`
}

// Backfill looks up every holding that still wants a profile.
//
// Requests go one at a time with a pause between them: providers rate-limit,
// and a backfill is a background chore with no one waiting on it. A failure
// stops nothing - it's counted and the pass moves on, so one bad symbol can't
// strand the rest.
func (e *Enricher) Backfill(ctx context.Context, db *gorm.DB, pause time.Duration) (BackfillResult, error) {
	var result BackfillResult
	if !e.Enabled() {
		return result, nil
	}

	var holdings []models.Holding
	if err := db.Where("profile_fetched_at IS NULL AND symbol <> ''").Find(&holdings).Error; err != nil {
		return result, err
	}

	for i := range holdings {
		h := &holdings[i]
		if !h.WantsProfile() {
			continue
		}
		result.Considered++

		if err := ctx.Err(); err != nil {
			return result, err
		}
		if result.Considered > 1 && pause > 0 {
			time.Sleep(pause)
		}

		before := h.Sector
		if err := e.Enrich(ctx, db, h); err != nil {
			log.Printf("market data: backfill for %s failed: %v", h.Symbol, err)
			result.Failed++
			continue
		}
		if h.Sector != before || h.CompanyName != "" {
			result.Enriched++
		} else {
			result.NotFound++
		}
	}
	return result, nil
}
