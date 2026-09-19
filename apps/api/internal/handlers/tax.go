package handlers

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/tax"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TaxHandler interface {
	GetJurisdictions(c *gin.Context)
	GetRules(c *gin.Context)
	GetSummary(c *gin.Context)
	GetEvents(c *gin.Context)
	GetProfiles(c *gin.Context)
	PutProfile(c *gin.Context)
	Recompute(c *gin.Context)
}

type taxHandler struct {
	db *gorm.DB
}

func NewTaxHandler(db *gorm.DB) TaxHandler {
	return &taxHandler{db: db}
}

// characterLabels are the human-readable names for tax characters. Labels live
// here rather than in the client so a jurisdiction added as data arrives with
// readable buckets instead of raw enum values.
var characterLabels = map[string]string{
	models.CharacterLongTermCapital:   "Long-term capital gains",
	models.CharacterShortTermCapital:  "Short-term capital gains",
	models.CharacterQualifiedEligible: "Dividends",
	models.CharacterOrdinary:          "Ordinary income",
	models.CharacterExempt:            "Exempt",
	models.CharacterDeferred:          "Tax-deferred",
}

// reasonLabels name why an amount was excluded from a jurisdiction's taxable
// income, so the UI can say what was left out rather than just showing a
// smaller total.
var reasonLabels = map[string]string{
	"federal_obligation_state_exempt":   "U.S. Treasury interest",
	"municipal_interest_federal_exempt": "Municipal bond interest",
	"in_state_municipal_exempt":         "In-state municipal interest",
	models.ReasonShelteredAccount:       "Tax-advantaged accounts",
}

// characterOrder sorts buckets so a card reads in a stable, sensible order
// rather than by whatever the group-by returned.
var characterOrder = map[string]int{
	models.CharacterLongTermCapital:   0,
	models.CharacterShortTermCapital:  1,
	models.CharacterQualifiedEligible: 2,
	models.CharacterOrdinary:          3,
}

func label(m map[string]string, key, fallback string) string {
	if v, ok := m[key]; ok {
		return v
	}
	if key == "" {
		return fallback
	}
	return key
}

type taxBucket struct {
	Character string  `json:"character"`
	Label     string  `json:"label"`
	Amount    float64 `json:"amount"`
} // @name TaxBucket

type taxExclusion struct {
	Reason string  `json:"reason"`
	Label  string  `json:"label"`
	Amount float64 `json:"amount"`
} // @name TaxExclusion

// capitalSummary is a jurisdiction's capital gains after netting, with the
// intermediate figures kept so the rule is visible rather than implied.
type capitalSummary struct {
	ShortTerm float64 `json:"short_term"`
	LongTerm  float64 `json:"long_term"`
	// Offset is how much of one holding period's loss the other's gain absorbed.
	Offset float64 `json:"offset"`
	Net    float64 `json:"net"`
	// Taxable is what reaches taxable income: the net when it's a gain, zero
	// when it's a loss.
	Taxable float64 `json:"taxable"`
	// LossCarryforward is a net loss left over, as a positive number.
	LossCarryforward float64 `json:"loss_carryforward"`
} // @name CapitalSummary

type jurisdictionSummary struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Level string `json:"level"`

	// Capital and Ordinary are reported apart because they are taxed apart: a
	// capital loss nets against capital gains only, never against interest or
	// dividends.
	Capital       capitalSummary `json:"capital"`
	Ordinary      []taxBucket    `json:"ordinary"`
	OrdinaryTotal float64        `json:"ordinary_total"`

	TaxableTotal float64        `json:"taxable_total"`
	Excluded     []taxExclusion `json:"excluded"`
} // @name JurisdictionSummary

type taxSummary struct {
	Year          int                   `json:"year"`
	Jurisdictions []jurisdictionSummary `json:"jurisdictions"`
} // @name TaxSummary

// GetSummary godoc
// @Summary      Taxable income for a year, broken down per jurisdiction
// @Tags         tax
// @Produce      json
// @Param        year  query     int  false  "Tax year (defaults to the current year)"
// @Success      200   {object}  taxSummary
// @Failure      500   {object}  map[string]string
// @Router       /tax/summary [get]
//
// The breakdown comes from the stored treatments, so each jurisdiction reports
// its own buckets - the U.S. splits capital gains by holding period while
// California folds them into ordinary rates, and neither shape is hardcoded.
//
// Capital gains are netted before they reach the total (see tax.CapitalPosition)
// and ordinary income is added separately, because a capital loss cannot reduce
// interest or dividends.
func (h *taxHandler) GetSummary(c *gin.Context) {
	year := time.Now().UTC().Year()
	if y := c.Query("year"); y != "" {
		if parsed, err := strconv.Atoi(y); err == nil {
			year = parsed
		}
	}

	// Netting is decided by what the income IS, which lives on the event, not by
	// the character it's taxed at - California taxes capital gains at ordinary
	// rates while still netting them as capital.
	var rows []struct {
		JurisdictionCode string
		Category         string
		Term             string
		TaxCharacter     string
		Amount           float64
	}
	if err := h.db.Table("tax_treatments AS t").
		Joins("JOIN realized_events AS e ON e.id = t.realized_event_id").
		Select("t.jurisdiction_code, e.category, e.term, t.tax_character, COALESCE(SUM(t.taxable_amount), 0) AS amount").
		Where("t.tax_year = ? AND t.taxable", year).
		Group("t.jurisdiction_code, e.category, e.term, t.tax_character").
		Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize tax treatments"})
		return
	}

	// What each jurisdiction excluded, and why.
	var exclusionRows []struct {
		JurisdictionCode string
		Reason           string
		Amount           float64
	}
	if err := h.db.Model(&models.TaxTreatment{}).
		Select("jurisdiction_code, reason, COALESCE(SUM(excluded_amount), 0) AS amount").
		Where("tax_year = ? AND NOT taxable", year).
		Group("jurisdiction_code, reason").
		Scan(&exclusionRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize tax treatments"})
		return
	}

	var jurisdictions []models.TaxJurisdiction
	if err := h.db.Find(&jurisdictions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch jurisdictions"})
		return
	}

	type accum struct {
		summary  *jurisdictionSummary
		capital  tax.CapitalPosition
		ordinary map[string]float64
		active   bool
	}
	byCode := map[string]*accum{}
	for _, j := range jurisdictions {
		byCode[j.Code] = &accum{
			summary: &jurisdictionSummary{
				Code: j.Code, Name: j.Name, Level: j.Level,
				Ordinary: []taxBucket{}, Excluded: []taxExclusion{},
			},
			ordinary: map[string]float64{},
		}
	}

	for _, r := range rows {
		a, ok := byCode[r.JurisdictionCode]
		if !ok {
			continue
		}
		a.active = true
		if r.Category != models.IncomeTypeCapitalGain {
			a.ordinary[r.TaxCharacter] += r.Amount
			continue
		}
		if r.Term == "long" {
			a.capital.LongTerm += r.Amount
		} else {
			a.capital.ShortTerm += r.Amount
		}
	}

	for _, r := range exclusionRows {
		a, ok := byCode[r.JurisdictionCode]
		if !ok || r.Amount == 0 {
			continue
		}
		a.active = true
		a.summary.Excluded = append(a.summary.Excluded, taxExclusion{
			Reason: r.Reason,
			Label:  label(reasonLabels, r.Reason, "Excluded"),
			Amount: r.Amount,
		})
	}

	// National level first, so Federal reads before State.
	out := make([]jurisdictionSummary, 0, len(byCode))
	for _, j := range jurisdictions {
		a := byCode[j.Code]
		if !a.active {
			continue
		}
		js := a.summary

		js.Capital = capitalSummary{
			ShortTerm:        a.capital.ShortTerm,
			LongTerm:         a.capital.LongTerm,
			Offset:           a.capital.Offset(),
			Net:              a.capital.Net(),
			Taxable:          a.capital.Taxable(),
			LossCarryforward: a.capital.LossCarryforward(),
		}

		for character, amount := range a.ordinary {
			js.Ordinary = append(js.Ordinary, taxBucket{
				Character: character,
				Label:     label(characterLabels, character, "Other"),
				Amount:    amount,
			})
			js.OrdinaryTotal += amount
		}
		sort.SliceStable(js.Ordinary, func(x, y int) bool {
			return characterOrder[js.Ordinary[x].Character] < characterOrder[js.Ordinary[y].Character]
		})
		sort.SliceStable(js.Excluded, func(x, y int) bool { return js.Excluded[x].Amount > js.Excluded[y].Amount })

		js.TaxableTotal = js.Capital.Taxable + js.OrdinaryTotal
		out = append(out, *js)
	}
	sort.SliceStable(out, func(a, b int) bool {
		return out[a].Level == models.JurisdictionLevelNational && out[b].Level != models.JurisdictionLevelNational
	})

	c.JSON(http.StatusOK, taxSummary{Year: year, Jurisdictions: out})
}

// GetJurisdictions godoc
// @Summary      List the tax jurisdictions the app knows about
// @Tags         tax
// @Produce      json
// @Success      200  {array}   models.TaxJurisdiction
// @Failure      500  {object}  map[string]string
// @Router       /tax/jurisdictions [get]
func (h *taxHandler) GetJurisdictions(c *gin.Context) {
	var jurisdictions []models.TaxJurisdiction
	if err := h.db.Order("level ASC").Order("code ASC").Find(&jurisdictions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch jurisdictions"})
		return
	}
	c.JSON(http.StatusOK, jurisdictions)
}

// GetRules godoc
// @Summary      List tax rules, optionally for one jurisdiction
// @Tags         tax
// @Produce      json
// @Param        jurisdiction  query     string  false  "Filter to one jurisdiction code (e.g. US-CA)"
// @Success      200           {array}   models.TaxRule
// @Failure      500           {object}  map[string]string
// @Router       /tax/rules [get]
func (h *taxHandler) GetRules(c *gin.Context) {
	q := h.db.Order("jurisdiction_code ASC").Order("priority DESC")
	if j := c.Query("jurisdiction"); j != "" {
		q = q.Where("jurisdiction_code = ?", j)
	}

	var rules []models.TaxRule
	if err := q.Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax rules"})
		return
	}
	c.JSON(http.StatusOK, rules)
}

// Recompute godoc
// @Summary      Re-evaluate every stored tax treatment against the current rules
// @Tags         tax
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /tax/recompute [post]
//
// Treatments are written when an event is realized, so editing a rule doesn't
// change events already recorded. This replays them.
func (h *taxHandler) Recompute(c *gin.Context) {
	if err := h.db.Transaction(tax.RecomputeAll); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to recompute tax treatments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type taxProfileRequest struct {
	UserID      string  `json:"user_id" binding:"required"`
	CountryCode string  `json:"country_code" binding:"required"`
	RegionCode  *string `json:"region_code"`
} // @name TaxProfileRequest

// GetProfiles godoc
// @Summary      List tax residency profiles
// @Tags         tax
// @Produce      json
// @Success      200  {array}   models.TaxProfile
// @Failure      500  {object}  map[string]string
// @Router       /tax/profiles [get]
func (h *taxHandler) GetProfiles(c *gin.Context) {
	var profiles []models.TaxProfile
	if err := h.db.Find(&profiles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax profiles"})
		return
	}
	c.JSON(http.StatusOK, profiles)
}

// PutProfile godoc
// @Summary      Set a user's tax residency
// @Tags         tax
// @Accept       json
// @Produce      json
// @Param        profile  body      taxProfileRequest  true  "Tax profile payload"
// @Success      200      {object}  models.TaxProfile
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /tax/profiles [put]
//
// Residency decides which jurisdictions a person's income is evaluated against,
// so changing it changes the answer for everything already realized - the
// treatments are rebuilt in the same transaction.
func (h *taxHandler) PutProfile(c *gin.Context) {
	var req taxProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Both codes must name jurisdictions we know about, or the profile resolves
	// to rules that don't exist and every event silently falls through to the
	// ordinary-income default.
	codes := []string{req.CountryCode}
	if req.RegionCode != nil && *req.RegionCode != "" {
		codes = append(codes, *req.RegionCode)
	}
	var known int64
	if err := h.db.Model(&models.TaxJurisdiction{}).Where("code IN ?", codes).Count(&known).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate jurisdictions"})
		return
	}
	if int(known) != len(codes) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown jurisdiction code"})
		return
	}

	var profile models.TaxProfile
	err = h.db.Where("user_id = ?", userID).First(&profile).Error
	switch {
	case err == nil:
	case errors.Is(err, gorm.ErrRecordNotFound):
		profile = models.TaxProfile{UserID: userID, EffectiveFrom: time.Now().UTC()}
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax profile"})
		return
	}

	profile.CountryCode = req.CountryCode
	profile.RegionCode = req.RegionCode
	if profile.EffectiveFrom.IsZero() {
		profile.EffectiveFrom = time.Now().UTC()
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&profile).Error; err != nil {
			return err
		}
		return tax.RecomputeAll(tx)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save tax profile"})
		return
	}

	c.JSON(http.StatusOK, profile)
}

// realizedEventSummary totals the year by what was realized. The split is by
// category rather than by tax bucket: how these amounts are taxed differs per
// jurisdiction and is reported by GET /tax/summary, while this is the ledger.
type realizedEventSummary struct {
	Total        float64 `json:"total"`
	CapitalGains float64 `json:"capital_gains"`
	Interest     float64 `json:"interest"`
	Dividends    float64 `json:"dividends"`
} // @name RealizedEventSummary

type paginatedRealizedEvents struct {
	Data     []models.RealizedEvent `json:"data"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Summary  realizedEventSummary   `json:"summary"`
} // @name PaginatedRealizedEvents

// GetEvents godoc
// @Summary      List everything realized in a tax year
// @Tags         tax
// @Produce      json
// @Param        year        query     int     false  "Tax year; omit for all years"
// @Param        category    query     string  false  "capital_gain | interest | dividend"
// @Param        origin      query     string  false  "disposal | distribution"
// @Param        symbol      query     string  false  "Filter to one security"
// @Param        account_id  query     string  false  "Filter to one account"
// @Param        holding_id  query     string  false  "Filter to one holding"
// @Param        page        query     int     false  "Page number (default 1)"
// @Param        page_size   query     int     false  "Items per page (default 20, max 100)"
// @Success      200         {object}  paginatedRealizedEvents
// @Failure      400         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /tax/events [get]
//
// Capital gains, Treasury interest, and dividends all live in one ledger, so
// filtering, sorting, and paging are a plain query over a single table rather
// than a union of the two ledgers this used to read.
func (h *taxHandler) GetEvents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Each filter is optional and they compose; applied identically to the
	// count, the summary, and the page so all three agree.
	var filterErr error
	filter := func(q *gorm.DB) *gorm.DB {
		if y := c.Query("year"); y != "" {
			if year, err := strconv.Atoi(y); err == nil {
				q = q.Where("EXTRACT(YEAR FROM event_date) = ?", year)
			}
		}
		if v := c.Query("category"); v != "" {
			q = q.Where("category = ?", v)
		}
		if v := c.Query("origin"); v != "" {
			q = q.Where("origin = ?", v)
		}
		if v := c.Query("symbol"); v != "" {
			q = q.Where("symbol = ?", v)
		}
		for param, column := range map[string]string{"account_id": "account_id", "holding_id": "holding_id"} {
			if v := c.Query(param); v != "" {
				id, err := uuid.Parse(v)
				if err != nil {
					filterErr = err
					continue
				}
				q = q.Where(column+" = ?", id)
			}
		}
		return q
	}

	var total int64
	if err := filter(h.db.Model(&models.RealizedEvent{})).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch realized events"})
		return
	}
	if filterErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id filter"})
		return
	}

	var summary realizedEventSummary
	if err := filter(h.db.Model(&models.RealizedEvent{})).
		Select("COALESCE(SUM(amount), 0) AS total, " +
			"COALESCE(SUM(amount) FILTER (WHERE category = 'capital_gain'), 0) AS capital_gains, " +
			"COALESCE(SUM(amount) FILTER (WHERE category = 'interest'), 0) AS interest, " +
			"COALESCE(SUM(amount) FILTER (WHERE category = 'dividend'), 0) AS dividends").
		Scan(&summary).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize realized events"})
		return
	}

	// Treatments are a real association now, so they preload rather than being
	// stitched on by hand.
	var events []models.RealizedEvent
	if err := filter(h.db.Preload("Treatments")).
		Order("event_date DESC").Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch realized events"})
		return
	}

	c.JSON(http.StatusOK, paginatedRealizedEvents{
		Data:     events,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Summary:  summary,
	})
}
