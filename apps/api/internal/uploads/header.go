package uploads

import (
	"fmt"
	"strings"
)

// A Fidelity positions export and a transactions export both put Symbol in
// column 2 and Description in column 3, so each parses the other's rows without
// complaint - silently producing holdings with no prices, or no transactions at
// all. The columns they don't share are what tells them apart, so every import
// checks for its own header before reading a row.

// headerMarkers are column names that appear in one export and not the other.
var (
	holdingsHeaderMarkers     = []string{"last price", "current value"}
	transactionsHeaderMarkers = []string{"run date", "action"}
)

// matchesHeader reports whether a row is the header of the expected export.
func matchesHeader(record []string, markers []string) bool {
	joined := strings.ToLower(strings.Join(record, ","))
	for _, m := range markers {
		if !strings.Contains(joined, m) {
			return false
		}
	}
	return true
}

// wrongFileError names what was uploaded and what it looks like, since the
// mistake is almost always the file-type selector rather than the file.
func wrongFileError(expected string) error {
	other := "transactions"
	if expected == "transactions" {
		other = "holdings"
	}
	return fmt.Errorf(
		"this doesn't look like a %s export - no %s header row found. "+
			"If it's a %s file, upload it as %s instead",
		expected, expected, other, other)
}
