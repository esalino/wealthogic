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

// Fidelity does not export one fixed set of columns. A brokerage account's
// transaction history carries currency and exchange-rate columns that a
// retirement account's history omits entirely, and the money columns are
// labeled "Amount ($)" in the shorter export and "Amount" in the longer one.
// Reading by position therefore can't work across accounts, so an import finds
// its columns by name.

// normalizeColumn reduces a header cell to a stable key: case, surrounding
// space, the "($)" unit suffix, and the UTF-8 BOM that opens an export are all
// presentation, not identity.
func normalizeColumn(s string) string {
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "($)", "")
	return strings.Join(strings.Fields(s), " ")
}

// columnIndex maps a normalized header name to its position in a row.
type columnIndex map[string]int

// indexColumns reads a header row into a lookup. The first occurrence of a name
// wins; a repeated label is never the one meant.
func indexColumns(record []string) columnIndex {
	cols := make(columnIndex, len(record))
	for i, cell := range record {
		name := normalizeColumn(cell)
		if name == "" {
			continue
		}
		if _, seen := cols[name]; !seen {
			cols[name] = i
		}
	}
	return cols
}

// optionalColumn returns the position of a column an export may not carry at
// all, or -1 when it doesn't.
func optionalColumn(cols columnIndex, name string) int {
	if i, ok := cols[name]; ok {
		return i
	}
	return -1
}

// field reads a column that may be absent from this export (index -1) or past
// the end of a short row, yielding the empty string rather than panicking -
// which is already what the parsers treat as "no value".
func field(record []string, idx int) string {
	if idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}
