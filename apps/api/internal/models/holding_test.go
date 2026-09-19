package models

import "testing"

// Asset class is what makes an allocation legible: sector only means something
// within equities, so cash and bonds have to be separated out before a sector
// breakdown says anything useful about the stocks.
func TestResolveAssetClass(t *testing.T) {
	govBond := TaxClassGovernmentBond
	tests := []struct {
		name    string
		holding Holding
		want    string
	}{
		{"a stock is equity", Holding{AssetType: AssetTypeStock}, AssetClassEquity},
		{"so is an ordinary ETF", Holding{AssetType: AssetTypeETF}, AssetClassEquity},
		{"and a mutual fund", Holding{AssetType: AssetTypeMutualFund}, AssetClassEquity},
		{"a treasury is fixed income", Holding{AssetType: AssetTypeTreasury}, AssetClassFixedIncome},
		{"a corporate bond too", Holding{AssetType: AssetTypeBond}, AssetClassFixedIncome},
		{"money market is cash", Holding{AssetType: AssetTypeMoneyMarket}, AssetClassCash},
		{"an option is a derivative", Holding{AssetType: AssetTypeOption}, AssetClassDerivatives},
		{
			// The case that makes deriving from tax class worth it: a bond ETF
			// is equity by wrapper and fixed income by exposure, and the tax
			// class already records which.
			name:    "a treasury ETF follows its tax class, not its wrapper",
			holding: Holding{AssetType: AssetTypeETF, TaxClassOverride: &govBond},
			want:    AssetClassFixedIncome,
		},
		{"anything unrecognized falls to other", Holding{AssetType: "Crypto"}, AssetClassOther},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.holding.ResolveAssetClass(); got != tc.want {
				t.Errorf("ResolveAssetClass() = %q, want %q", got, tc.want)
			}
		})
	}
}
