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

// Apply writes the per-jurisdiction treatments for one realized event. holding
// is the security behind it, or nil when it isn't tracked.
//
// One ledger means one entry point: a disposal and a dividend differ only in the
// attributes they carry into the rules, not in how they're evaluated or stored.
func (a *Applier) Apply(ev *models.RealizedEvent, holding *models.Holding) error {
	assetClass := models.TaxClassOther
	issuer := ""
	if holding != nil {
		assetClass = holding.ResolveTaxClass()
		issuer = holding.ResolveIssuerJurisdiction()
	}

	// Holding period has no bearing on anything but a capital gain, and leaving
	// it set would let a term-matching rule catch income it was never meant to.
	term := ev.Term
	if ev.Category != models.IncomeTypeCapitalGain {
		term = ""
	}

	ctx, err := a.contextFor(ev.AccountID)
	if err != nil {
		return err
	}

	// Replace rather than upsert: the jurisdiction set itself can change (a move,
	// an edited profile), which leaves stale rows an upsert would never touch.
	if err := a.tx.Where("realized_event_id = ?", ev.ID).Delete(&models.TaxTreatment{}).Error; err != nil {
		return err
	}

	treatments := Evaluate(Event{
		RealizedEventID:    ev.ID,
		IncomeType:         ev.Category,
		Amount:             ev.Amount,
		Term:               term,
		AssetTaxClass:      assetClass,
		IssuerJurisdiction: issuer,
		TaxYear:            ev.EventDate.Year(),
	}, ctx)
	if len(treatments) == 0 {
		return nil
	}
	return a.tx.Create(&treatments).Error
}

// RecomputeAll rebuilds every treatment from the current rules. Exposed on the
// API because editing a rule changes the answer for events already realized.
//
// It re-evaluates rather than re-derives: the events themselves come from their
// source records and are rebuilt by the portfolio package.
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

	var events []models.RealizedEvent
	if err := tx.Find(&events).Error; err != nil {
		return err
	}
	for i := range events {
		// Resolve the security by id, falling back to its symbol for events
		// recorded before a matching holding existed.
		var holding *models.Holding
		if events[i].HoldingID != nil {
			holding = byID[*events[i].HoldingID]
		}
		if holding == nil {
			holding = bySymbol[events[i].Symbol]
		}
		if err := applier.Apply(&events[i], holding); err != nil {
			return err
		}
	}
	return nil
}
