package models

import "testing"

func TestParseOptionSymbol(t *testing.T) {
	tests := []struct {
		symbol     string
		underlying string
		expiry     string
		optType    string
		strike     float64
		ok         bool
	}{
		{"-AXP251121C390", "AXP", "2025-11-21", OptionTypeCall, 390, true},
		{"-SLDP250815P2.5", "SLDP", "2025-08-15", OptionTypePut, 2.5, true},
		{"-CRCL250620C230", "CRCL", "2025-06-20", OptionTypeCall, 230, true},
		{"-TSLA250516C372.5", "TSLA", "2025-05-16", OptionTypeCall, 372.5, true},
		{"-MAR260717P320", "MAR", "2026-07-17", OptionTypePut, 320, true},
		// Not options - plain tickers and a CUSIP must pass through untouched.
		{"AXP", "", "", "", 0, false},
		{"912797RS8", "", "", "", 0, false},
		{"FZFXX", "", "", "", 0, false},
	}
	for _, tc := range tests {
		got, ok := ParseOptionSymbol(tc.symbol)
		if ok != tc.ok {
			t.Errorf("%s: ok=%v want %v", tc.symbol, ok, tc.ok)
			continue
		}
		if !tc.ok {
			continue
		}
		if got.Underlying != tc.underlying || got.Type != tc.optType || got.Strike != tc.strike ||
			got.Expiration.Format("2006-01-02") != tc.expiry {
			t.Errorf("%s: got %+v", tc.symbol, got)
		}
	}
}
