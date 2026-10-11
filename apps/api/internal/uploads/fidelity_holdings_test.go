package uploads

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

// Rows as a Fidelity positions export writes them. The pending row is shorter
// than a position row, which is what once got it discarded.
const positionsSample = `Account number,Account name,Symbol,Description,Quantity,Last price,Last price change,Current value,Today's gain/loss dollar,Today's gain/loss percent,Total gain/loss dollar,Total gain/loss percent,Percent of account,Cost basis total,Average cost basis,Type
Z07831766,Individual - TOD,FZFXX**,HELD IN MONEY MARKET,,,,$361803.94,,,,,51.75%,,,Cash,
Z07831766,Individual - TOD,91282CRQ6,UNITED STATES TREAS SER AT-2029 4.87500% 10/15/2029 NTS NOTE,50000,$99.875,-$0.086,$49937.50,-$43.00,-0.09%,+$16.08,+0.03%,7.14%,$49921.42,--,Cash,
Z07831766,Individual - TOD,Pending activity,,,,,-$49921.42,,,,,,

"Date downloaded Oct-11-2026 7:04 p.m ET"
`

func readSample(t *testing.T) [][]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(positionsSample))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	return records
}

func TestPendingActivity(t *testing.T) {
	var found []float64
	for _, record := range readSample(t) {
		if v, ok := pendingActivity(record); ok {
			found = append(found, v)
		}
	}
	if len(found) != 1 || found[0] != -49921.42 {
		t.Fatalf("pendingActivity found %v, want [-49921.42]", found)
	}
}

func TestDownloadedDate(t *testing.T) {
	var found []time.Time
	for _, record := range readSample(t) {
		if d, ok := downloadedDate(record); ok {
			found = append(found, d)
		}
	}
	want := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	if len(found) != 1 || !found[0].Equal(want) {
		t.Fatalf("downloadedDate found %v, want [%v]", found, want)
	}
}
