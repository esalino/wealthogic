package tax

import (
	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DefaultProfile is the residency assumed for an account with no owner carrying
// a tax profile. Imports create accounts and transactions without necessarily
// knowing who owns them, and an event with no treatment at all would silently
// vanish from the Tax Center - so fall back rather than fail.
var DefaultProfile = models.TaxProfile{
	CountryCode: models.JurisdictionUS,
	RegionCode:  ptr(models.JurisdictionUSCA),
}

func ptr[T any](v T) *T { return &v }

// Applier writes tax treatments for realized events. It caches the jurisdiction
// tree, the rules, and per-account residency for the life of one operation,
// since a single import or holding rebuild evaluates many events against the
// same small set of rules.
//
// It is not safe for concurrent use, and is scoped to the caller's transaction:
// treatments are written through the same tx as the events they describe.
type Applier struct {
	tx    *gorm.DB
	tree  *Tree
	rules *RuleSet

	// Per-account context, resolved on first use.
	jurisdictions map[uuid.UUID][]string
	taxTypes      map[uuid.UUID]string
}

// NewApplier loads the rules and jurisdiction tree once for a batch of events.
func NewApplier(tx *gorm.DB) (*Applier, error) {
	tree, err := LoadTree(tx)
	if err != nil {
		return nil, err
	}
	rules, err := LoadRuleSet(tx)
	if err != nil {
		return nil, err
	}
	return &Applier{
		tx:            tx,
		tree:          tree,
		rules:         rules,
		jurisdictions: map[uuid.UUID][]string{},
		taxTypes:      map[uuid.UUID]string{},
	}, nil
}

// contextFor resolves an account's shelter and the jurisdictions its activity is
// taxed in. Residency belongs to the owner, so it comes from the account's
// owners' tax profiles, falling back to DefaultProfile.
func (a *Applier) contextFor(accountID uuid.UUID) (Context, error) {
	if codes, ok := a.jurisdictions[accountID]; ok {
		return Context{
			JurisdictionCodes: codes,
			AccountTaxType:    a.taxTypes[accountID],
			Rules:             a.rules,
			Tree:              a.tree,
		}, nil
	}

	var account models.Account
	if err := a.tx.Preload("Owners").First(&account, "id = ?", accountID).Error; err != nil {
		// An event on an account we can't read still deserves a treatment;
		// assume the default residency and no shelter.
		if err != gorm.ErrRecordNotFound {
			return Context{}, err
		}
		account = models.Account{TaxType: models.AccountTaxTypePersonal}
	}

	profile := DefaultProfile
	if len(account.Owners) > 0 {
		ownerIDs := make([]uuid.UUID, 0, len(account.Owners))
		for _, o := range account.Owners {
			ownerIDs = append(ownerIDs, o.ID)
		}
		// Most recent residency among the owners. Joint accounts with owners in
		// different jurisdictions aren't modeled; the first profile wins.
		var found models.TaxProfile
		err := a.tx.Where("user_id IN ?", ownerIDs).Order("effective_from DESC").First(&found).Error
		switch {
		case err == nil:
			profile = found
		case err != gorm.ErrRecordNotFound:
			return Context{}, err
		}
	}

	codes := profile.JurisdictionCodes()
	a.jurisdictions[accountID] = codes
	a.taxTypes[accountID] = account.TaxType

	return Context{
		JurisdictionCodes: codes,
		AccountTaxType:    account.TaxType,
		Rules:             a.rules,
		Tree:              a.tree,
	}, nil
}

// ApplyToGain writes the per-jurisdiction treatments for one realized capital
// gain. holding is the disposed security, or nil when it isn't tracked.
func (a *Applier) ApplyToGain(g *models.Gain, holding *models.Holding) error {
	assetClass := models.TaxClassOther
	issuer := ""
	if holding != nil {
		assetClass = holding.ResolveTaxClass()
		issuer = holding.ResolveIssuerJurisdiction()
	}

	return a.apply(Event{
		SourceType:         models.TaxSourceGain,
		SourceID:           g.ID,
		IncomeType:         models.IncomeTypeCapitalGain,
		Amount:             g.Amount,
		Term:               g.Term,
		AssetTaxClass:      assetClass,
		IssuerJurisdiction: issuer,
		TaxYear:            g.RealizedDate.Year(),
		AccountID:          g.AccountID,
		HoldingID:          g.HoldingID,
	})
}

// ApplyToDistribution writes the per-jurisdiction treatments for one income
// payment. holding is the paying security, or nil when it isn't tracked.
func (a *Applier) ApplyToDistribution(d *models.Distribution, holding *models.Holding) error {
	incomeType := models.IncomeTypeDividend
	if d.Category == models.IncomeTypeInterest {
		incomeType = models.IncomeTypeInterest
	}

	return a.apply(Event{
		SourceType:         models.TaxSourceDistribution,
		SourceID:           d.ID,
		IncomeType:         incomeType,
		Amount:             d.Amount,
		AssetTaxClass:      d.ResolveTaxClass(holding),
		IssuerJurisdiction: d.ResolveIssuerJurisdiction(holding),
		TaxYear:            d.PaymentDate.Year(),
		AccountID:          d.AccountID,
		HoldingID:          d.HoldingID,
	})
}

// apply evaluates an event and replaces its stored treatments.
func (a *Applier) apply(ev Event) error {
	ctx, err := a.contextFor(ev.AccountID)
	if err != nil {
		return err
	}

	// Replace rather than upsert: the jurisdiction set itself can change (a move,
	// an edited profile), which leaves stale rows an upsert would never touch.
	if err := a.tx.Where("source_type = ? AND source_id = ?", ev.SourceType, ev.SourceID).
		Delete(&models.TaxTreatment{}).Error; err != nil {
		return err
	}

	treatments := Evaluate(ev, ctx)
	if len(treatments) == 0 {
		return nil
	}
	return a.tx.Create(&treatments).Error
}

// DeleteForHolding removes every treatment belonging to a holding's events. The
// gain ledger is rebuilt from scratch when a holding's trades change; its
// treatments have to go with it or they outlive the rows they describe.
func DeleteForHolding(tx *gorm.DB, holdingID uuid.UUID) error {
	return tx.Where("holding_id = ? AND source_type = ?", holdingID, models.TaxSourceGain).
		Delete(&models.TaxTreatment{}).Error
}

// RecomputeHolding re-evaluates every treatment for a holding's gains and
// distributions, for when the holding's own tax attributes change - a tax class
// or issuer edit changes the answer for events already realized.
func RecomputeHolding(tx *gorm.DB, holdingID uuid.UUID) error {
	applier, err := NewApplier(tx)
	if err != nil {
		return err
	}

	var holding models.Holding
	if err := tx.First(&holding, "id = ?", holdingID).Error; err != nil {
		return err
	}

	var gains []models.Gain
	if err := tx.Where("holding_id = ?", holdingID).Find(&gains).Error; err != nil {
		return err
	}
	for i := range gains {
		if err := applier.ApplyToGain(&gains[i], &holding); err != nil {
			return err
		}
	}

	// A security's dividends can predate its first buy, so they carry the symbol
	// but a null holding_id - match on either, as the distributions handler does.
	var dists []models.Distribution
	q := tx.Where("holding_id = ?", holdingID)
	if holding.Symbol != "" {
		q = tx.Where("holding_id = ? OR symbol = ?", holdingID, holding.Symbol)
	}
	if err := q.Find(&dists).Error; err != nil {
		return err
	}
	for i := range dists {
		if err := applier.ApplyToDistribution(&dists[i], &holding); err != nil {
			return err
		}
	}
	return nil
}

// RecomputeAll rebuilds every treatment from the current rules. Called after the
// one-time migration and exposed on the API, since editing a rule changes the
// answer for events already realized.
func RecomputeAll(tx *gorm.DB) error {
	applier, err := NewApplier(tx)
	if err != nil {
		return err
	}

	var holdings []models.Holding
	if err := tx.Find(&holdings).Error; err != nil {
		return err
	}
	byID := make(map[uuid.UUID]*models.Holding, len(holdings))
	bySymbol := make(map[string]*models.Holding, len(holdings))
	for i := range holdings {
		byID[holdings[i].ID] = &holdings[i]
		if holdings[i].Symbol != "" {
			bySymbol[holdings[i].Symbol] = &holdings[i]
		}
	}

	// Resolve a security by its id, falling back to its symbol for rows recorded
	// before a matching holding existed.
	resolve := func(holdingID *uuid.UUID, symbol string) *models.Holding {
		if holdingID != nil {
			if h, ok := byID[*holdingID]; ok {
				return h
			}
		}
		return bySymbol[symbol]
	}

	var gains []models.Gain
	if err := tx.Find(&gains).Error; err != nil {
		return err
	}
	for i := range gains {
		if err := applier.ApplyToGain(&gains[i], resolve(gains[i].HoldingID, gains[i].Symbol)); err != nil {
			return err
		}
	}

	var dists []models.Distribution
	if err := tx.Find(&dists).Error; err != nil {
		return err
	}
	for i := range dists {
		if err := applier.ApplyToDistribution(&dists[i], resolve(dists[i].HoldingID, dists[i].Symbol)); err != nil {
			return err
		}
	}
	return nil
}
