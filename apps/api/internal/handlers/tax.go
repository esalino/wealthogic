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

type jurisdictionSummary struct {
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Level        string         `json:"level"`
	TaxableTotal float64        `json:"taxable_total"`
	Buckets      []taxBucket    `json:"buckets"`
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
// The breakdown comes entirely from the stored treatments, so each jurisdiction
// reports its own buckets: the U.S. splits capital gains by holding period while
// California folds them into ordinary income, and neither shape is hardcoded
// anywhere - a jurisdiction added as rule rows shows up here on its own.
func (h *taxHandler) GetSummary(c *gin.Context) {
	year := time.Now().UTC().Year()
	if y := c.Query("year"); y != "" {
		if parsed, err := strconv.Atoi(y); err == nil {
			year = parsed
		}
	}

	// Taxable income grouped by jurisdiction and character.
	var bucketRows []struct {
		JurisdictionCode string
		TaxCharacter     string
		Amount           float64
	}
	if err := h.db.Model(&models.TaxTreatment{}).
		Select("jurisdiction_code, tax_character, COALESCE(SUM(taxable_amount), 0) AS amount").
		Where("tax_year = ? AND taxable", year).
		Group("jurisdiction_code, tax_character").
		Scan(&bucketRows).Error; err != nil {
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

	byCode := map[string]*jurisdictionSummary{}
	for _, j := range jurisdictions {
		byCode[j.Code] = &jurisdictionSummary{
			Code:     j.Code,
			Name:     j.Name,
			Level:    j.Level,
			Buckets:  []taxBucket{},
			Excluded: []taxExclusion{},
		}
	}

	for _, r := range bucketRows {
		js, ok := byCode[r.JurisdictionCode]
		if !ok {
			continue
		}
		js.TaxableTotal += r.Amount
		js.Buckets = append(js.Buckets, taxBucket{
			Character: r.TaxCharacter,
			Label:     label(characterLabels, r.TaxCharacter, "Other"),
			Amount:    r.Amount,
		})
	}

	for _, r := range exclusionRows {
		js, ok := byCode[r.JurisdictionCode]
		if !ok || r.Amount == 0 {
			continue
		}
		js.Excluded = append(js.Excluded, taxExclusion{
			Reason: r.Reason,
			Label:  label(reasonLabels, r.Reason, "Excluded"),
			Amount: r.Amount,
		})
	}

	// Only report jurisdictions that actually have activity this year, national
	// level first so Federal reads before State.
	out := make([]jurisdictionSummary, 0, len(byCode))
	for _, j := range jurisdictions {
		js := byCode[j.Code]
		if len(js.Buckets) == 0 && len(js.Excluded) == 0 {
			continue
		}
		sort.SliceStable(js.Buckets, func(a, b int) bool {
			return characterOrder[js.Buckets[a].Character] < characterOrder[js.Buckets[b].Character]
		})
		sort.SliceStable(js.Excluded, func(a, b int) bool { return js.Excluded[a].Amount > js.Excluded[b].Amount })
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
