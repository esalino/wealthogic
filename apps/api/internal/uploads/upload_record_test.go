package uploads

import "testing"

// History names the file that was imported, not where the caller's shell was
// standing when it uploaded it.
func TestBaseFileName(t *testing.T) {
	tests := map[string]string{
		"../../Transactions_Erik_Fid_Roll_IRA_YTD_2026-10-03.csv": "Transactions_Erik_Fid_Roll_IRA_YTD_2026-10-03.csv",
		"Portfolio_Positions_Sep-13-2026.csv":                     "Portfolio_Positions_Sep-13-2026.csv",
		"/absolute/path/to/Accounts_History.csv":                  "Accounts_History.csv",
		`C:\Users\erik\Downloads\Positions.csv`:                   "Positions.csv",
		"  ./Positions.csv  ":                                     "Positions.csv",
		"":                                                        "",
	}
	for in, want := range tests {
		if got := baseFileName(in); got != want {
			t.Errorf("baseFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

// A name that is nothing but separators has no basename to find; it must come
// back as-is rather than as the empty string, so history still shows something.
func TestBaseFileNameAllSeparators(t *testing.T) {
	if got := baseFileName("/"); got != "/" {
		t.Errorf(`baseFileName("/") = %q, want "/"`, got)
	}
}
