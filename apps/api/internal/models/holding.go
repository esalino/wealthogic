package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Holding status values. A holding is Open while any shares remain and Closed
// once the position has been fully sold off.
const (
	HoldingStatusOpen   = "Open"
	HoldingStatusClosed = "Closed"
)

// Asset types the tax rules single out. Kept here so the rules and the
// importer's classification agree on the spelling.
const (
	AssetTypeTreasury    = "Treasury"
	AssetTypeMoneyMarket = "Money Market"
	AssetTypeStock       = "Stock"
	AssetTypeETF         = "ETF"
	AssetTypeMutualFund  = "Mutual Fund"
	AssetTypeBond        = "Bond"
	AssetTypeOption      = "Option"
)

// defaultTaxClass is the tax class implied by an asset type alone. It mirrors
// the importer's asset-type classification (see uploads.assetTypeFor) so an
// imported holding lands in the right class without the user touching it.
func defaultTaxClass(assetType string) string {
	switch assetType {
	case AssetTypeTreasury:
		return TaxClassGovernmentBond
	case AssetTypeMoneyMarket:
		return TaxClassMoneyMarket
	case AssetTypeStock, AssetTypeETF, AssetTypeMutualFund:
		return TaxClassEquity
	case AssetTypeBond:
		return TaxClassCorporateBond
	}
	return TaxClassOther
}

// defaultIssuerJurisdiction is the issuing government implied by an asset type
// alone. Only government debt has one by default, and only a treasury's issuer
// is unambiguous from the asset type; a municipal bond's issuing state has to
// be set on the holding.
func defaultIssuerJurisdiction(assetType string) *string {
	if assetType == AssetTypeTreasury {
		code := JurisdictionUS
		return &code
	}
	return nil
}

// Holding represents a single holding. A holding can be made up of multiple child tax lots.
type Holding struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                                          json:"-"`

	AssetType   string `gorm:"not null"  json:"asset_type"`
	Symbol      string `json:"symbol"`
	Description string `gorm:"not null"  json:"description"`

	// Option detail, decoded from the contract symbol (see ParseOptionSymbol).
	// Nil/empty for everything else.
	Underlying     string     `gorm:"index" json:"underlying"`
	OptionType     string     `json:"option_type"` // "call" | "put"
	StrikePrice    *float64   `json:"strike_price"`
	ExpirationDate *time.Time `gorm:"type:date" json:"expiration_date"`

	// ContractMultiplier is how many shares one unit of this holding covers -
	// 100 for a standard option contract, 1 for everything else. Quoted prices
	// are per share, so every basis and proceeds figure scales by it.
	ContractMultiplier float64 `gorm:"not null;default:1" json:"contract_multiplier"`

	// Reference data about the security, from a market-data provider. Purely
	// descriptive - nothing in the portfolio or tax math reads it - so it is
	// filled in when it can be and left empty when it can't.
	CompanyName string `json:"company_name"`

	// CompanyDescription is the provider's prose summary of the business. It's
	// stored but not serialized: it runs to several paragraphs, and sending it
	// with every row of a holdings list would cost far more than it's worth
	// while nothing reads it. Drop the "-" tag to expose it.
	CompanyDescription string `gorm:"type:text" json:"-"`

	Sector   string `gorm:"index" json:"sector"`
	Industry string `gorm:"index" json:"industry"`
	Exchange string `json:"exchange"`
	Country  string `gorm:"size:8" json:"country"`
	Website  string `json:"website"`
	LogoURL  string `json:"logo_url"`

	// ProfileFetchedAt records that a lookup happened, whether or not it found
	// anything. Without it a symbol the provider doesn't cover - a treasury
	// CUSIP, an option contract - would be retried on every pass forever.
	ProfileFetchedAt *time.Time `json:"profile_fetched_at"`

	Status string `gorm:"not null;default:Open" json:"status"`

	LastPrice float64 `json:"last_price"`

	Quantity     float64 `gorm:"not null;default:0"  json:"purchase_quantity"`
	CurrentValue float64 `json:"current_value"`

	AverageCostBasis float64 `json:"average_cost_basis"`
	CostBasisTotal   float64 `gorm:"not null;default:0" json:"cost_basis_total"`

	GainUnrealizedPercent float64 `gorm:"not null;default:0" json:"gain_unrealized_percent"`
	GainUnrealizedAmount  float64 `gorm:"not null;default:0" json:"gain_unrealized_amount"`

	GainRealizedPercent float64 `json:"gain_realized_percent"`
	GainRealizedAmount  float64 `json:"gain_realized_amount"`
	DividendIncome      float64 `json:"dividend_income"`

	// TaxClassOverride overrides the tax class implied by the asset type. Nil
	// means "derive from the asset type" (see ResolveTaxClass); a non-nil value
	// is an explicit user choice - e.g. a treasury-only ETF like TLT classed as
	// government_bond even though ETFs are equity by default.
	TaxClassOverride *string `gorm:"size:32" json:"tax_class_override"`

	// IssuerJurisdiction is the government that issued this asset's debt, for
	// the bond classes. It's what the tax rules compare against a jurisdiction
	// to decide questions like "did my own state issue this?" - nil means the
	// asset-type default (see defaultIssuerJurisdiction).
	IssuerJurisdiction *string `gorm:"size:16" json:"issuer_jurisdiction"`

	// TaxClass is the resolved class (override, else the asset-type default),
	// computed for responses via AfterFind and never persisted. AssetClass is
	// the allocation slice it implies.
	TaxClass   string `gorm:"-" json:"tax_class"`
	AssetClass string `gorm:"-" json:"asset_class"`
} // @name Holding

// Multiplier is the shares one unit of this holding covers, defaulting to 1 for
// a holding recorded before the field existed.
func (h Holding) Multiplier() float64 {
	if h.ContractMultiplier > 0 {
		return h.ContractMultiplier
	}
	return 1
}

// ResolveTaxClass reports what kind of income this holding produces for tax
// purposes: the per-holding override when set, otherwise the asset-type default.
func (h Holding) ResolveTaxClass() string {
	if h.TaxClassOverride != nil && *h.TaxClassOverride != "" {
		return *h.TaxClassOverride
	}
	return defaultTaxClass(h.AssetType)
}

// ResolveIssuerJurisdiction reports which government issued this holding's debt,
// or "" when none is recorded (the usual case outside the bond classes).
func (h Holding) ResolveIssuerJurisdiction() string {
	if h.IssuerJurisdiction != nil && *h.IssuerJurisdiction != "" {
		return *h.IssuerJurisdiction
	}
	if code := defaultIssuerJurisdiction(h.AssetType); code != nil {
		return *code
	}
	return ""
}

// Asset classes: the top level of an allocation, above sector.
//
// Sector only means something within equities - a Treasury has no sector, and a
// money-market fund's provider-assigned "Financial Services" describes the fund
// company rather than the exposure. Splitting by class first is what makes the
// equity slice legible instead of buried under cash and bonds.
const (
	AssetClassEquity      = "Equity"
	AssetClassFixedIncome = "Fixed Income"
	AssetClassCash        = "Cash"
	AssetClassDerivatives = "Derivatives"
	AssetClassOther       = "Other"
)

// ResolveAssetClass reports which slice of an allocation this holding belongs
// to, derived from its tax class rather than a field of its own.
//
// The tax class already carries the distinction and is correctable per holding,
// so a Treasury-only ETF classed as government_bond lands in Fixed Income
// rather than Equity - which is what it is, whatever its wrapper.
func (h Holding) ResolveAssetClass() string {
	// An option's tax class is "other"; the asset type is what identifies it.
	if h.AssetType == AssetTypeOption {
		return AssetClassDerivatives
	}
	switch h.ResolveTaxClass() {
	case TaxClassEquity:
		return AssetClassEquity
	case TaxClassGovernmentBond, TaxClassMunicipalBond, TaxClassCorporateBond:
		return AssetClassFixedIncome
	case TaxClassMoneyMarket:
		return AssetClassCash
	}
	return AssetClassOther
}

// WantsProfile reports whether it's worth asking a market-data provider about
// this holding.
//
// Only securities with an issuer behind them have a profile: an option is a
// contract rather than a company, and a treasury is government debt identified
// by CUSIP. Asking anyway costs a rate-limited call to be told nothing.
func (h Holding) WantsProfile() bool {
	if h.Symbol == "" || h.ProfileFetchedAt != nil {
		return false
	}
	if h.AssetType == AssetTypeOption || h.AssetType == AssetTypeTreasury {
		return false
	}
	if _, isOption := ParseOptionSymbol(h.Symbol); isOption {
		return false
	}
	return true
}

// AfterFind populates the computed fields whenever a holding is read.
func (h *Holding) AfterFind(*gorm.DB) error {
	h.TaxClass = h.ResolveTaxClass()
	h.AssetClass = h.ResolveAssetClass()
	return nil
}
