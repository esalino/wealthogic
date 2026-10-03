package uploads

import "testing"

func qty(v float64) *float64 { return &v }

// mapAction carries the whole options model: which side of the market a trade
// lands on, and whether it opens or closes. Getting a sign backwards silently
// strands closes against lots they can never match, so the conventions are
// pinned here against the exact strings Fidelity exports.
func TestMapAction(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		quantity  *float64
		action    string
		effect    string
		direction string
	}{
		{
			name: "stock buy opens long",
			raw:  "YOU BOUGHT PROSHARES TR (UBER)", quantity: qty(100),
			action: actionBuy, effect: effectOpen, direction: directionLong,
		},
		{
			name: "stock sell closes long",
			raw:  "YOU SOLD PROSHARES TR (UBER)", quantity: qty(-100),
			action: actionSell, effect: effectClose, direction: directionLong,
		},
		{
			name: "treasury redemption closes long",
			raw:  "REDEMPTION PAYOUT (912797RS8)", quantity: qty(-50000),
			action: actionSell, effect: effectClose, direction: directionLong,
		},
		{
			name: "buying an option to open goes long",
			raw:  "YOU BOUGHT OPENING TRANSACTION CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $395 (100 SHS) (Margin)", quantity: qty(2),
			action: actionBuy, effect: effectOpen, direction: directionLong,
		},
		{
			name: "selling an option to close exits a long",
			raw:  "YOU SOLD CLOSING TRANSACTION CALL (CRCL) CIRCLE INTERNET JUN 20 25 $240 (100 SHS) (Margin)", quantity: qty(-3),
			action: actionSell, effect: effectClose, direction: directionLong,
		},
		{
			// The inversion: a sale that OPENS a position rather than closing one.
			name: "writing an option opens a short",
			raw:  "YOU SOLD OPENING TRANSACTION CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $390 (100 SHS) (Margin)", quantity: qty(-2),
			action: actionSell, effect: effectOpen, direction: directionShort,
		},
		{
			name: "buying an option to close covers a short",
			raw:  "YOU BOUGHT CLOSING TRANSACTION CALL (CRCL) CIRCLE INTERNET JUN 20 25 $230 (100 SHS) (Margin)", quantity: qty(3),
			action: actionBuy, effect: effectClose, direction: directionShort,
		},
		{
			// Fidelity records an expiration as the offsetting trade, so its sign
			// is opposite to the position it removes.
			name: "a long contract expiring is signed negative",
			raw:  "EXPIRED CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $395 as of 2025-11-21", quantity: qty(-2),
			action: actionExpired, effect: effectClose, direction: directionLong,
		},
		{
			name: "a written contract expiring is signed positive",
			raw:  "EXPIRED CALL (AXP) AMERICAN EXPRESS CO NOV 21 25 $390 as of 2025-11-21", quantity: qty(2),
			action: actionExpired, effect: effectClose, direction: directionShort,
		},
		{
			name: "a dividend touches no lot",
			raw:  "DIVIDEND RECEIVED (TLT)", quantity: nil,
			action: actionDividend, effect: "", direction: "",
		},
		{
			name: "an unmapped action passes through with no lot effect",
			raw:  "ELECTRONIC FUNDS TRANSFER RECEIVED", quantity: nil,
			action: "ELECTRONIC FUNDS TRANSFER RECEIVED", effect: "", direction: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action, effect, direction := mapAction(tc.raw, tc.quantity)
			if action != tc.action || effect != tc.effect || direction != tc.direction {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					action, effect, direction, tc.action, tc.effect, tc.direction)
			}
		})
	}
}

// The two Fidelity transaction layouts seen in real exports. The brokerage one
// carries currency and exchange-rate columns and labels money columns plainly;
// the retirement one drops those four columns - shifting everything after Type
// four places left - and suffixes money columns with "($)". Reading by fixed
// position silently skipped every row of the retirement export, so both
// layouts are pinned here.
var (
	brokerageTxnHeader = []string{
		"Run Date", "Action", "Symbol", "Description", "Type",
		"Exchange Quantity", "Exchange Currency", "Currency", "Price", "Quantity",
		"Exchange Rate", "Commission", "Fees", "Accrued Interest", "Amount",
		"Cash Balance", "Settlement Date",
	}
	retirementTxnHeader = []string{
		"Run Date", "Action", "Symbol", "Description", "Type", "Price ($)",
		"Quantity", "Commission ($)", "Fees ($)", "Accrued Interest ($)",
		"Amount ($)", "Cash Balance ($)", "Settlement Date",
	}
)

func TestResolveTxnColumns(t *testing.T) {
	tests := []struct {
		name   string
		header []string
		want   txnColumns
	}{
		{
			name:   "brokerage export",
			header: brokerageTxnHeader,
			want: txnColumns{
				runDate: 0, action: 1, symbol: 2, description: 3,
				price: 8, quantity: 9, commission: 11, fees: 12,
				amount: 14, settlementDate: 16, lastRequired: 14,
			},
		},
		{
			name:   "retirement export drops the currency columns",
			header: retirementTxnHeader,
			want: txnColumns{
				runDate: 0, action: 1, symbol: 2, description: 3,
				price: 5, quantity: 6, commission: 7, fees: 8,
				amount: 10, settlementDate: 12, lastRequired: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTxnColumns(tt.header)
			if err != nil {
				t.Fatalf("resolveTxnColumns() error = %v", err)
			}
			if *got != tt.want {
				t.Errorf("resolveTxnColumns() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

// An export missing a column the import can't do without must say so rather
// than import zero rows and report success, which is how the retirement export
// failed before columns were resolved by name.
func TestResolveTxnColumnsRequiresIdentifyingColumns(t *testing.T) {
	header := []string{"Run Date", "Action", "Symbol", "Description", "Type", "Price ($)"}
	if _, err := resolveTxnColumns(header); err == nil {
		t.Fatal("resolveTxnColumns() with no quantity or amount column: want error, got nil")
	}
}

// Optional columns are genuinely optional: an export without them still
// imports, and their absence reads as "no value" rather than panicking.
func TestResolveTxnColumnsOptionalColumnsAbsent(t *testing.T) {
	header := []string{"Run Date", "Action", "Symbol", "Description", "Quantity", "Amount"}
	cols, err := resolveTxnColumns(header)
	if err != nil {
		t.Fatalf("resolveTxnColumns() error = %v", err)
	}
	for name, idx := range map[string]int{
		"price": cols.price, "commission": cols.commission,
		"fees": cols.fees, "settlementDate": cols.settlementDate,
	} {
		if idx != -1 {
			t.Errorf("%s = %d, want -1", name, idx)
		}
	}
	row := []string{"08/25/2026", "YOU BOUGHT CHEVRON CORP NEW COM (CVX)", "CVX", "CHEVRON CORP NEW COM", "100", "-18032.99"}
	if got := field(row, cols.price); got != "" {
		t.Errorf("field(price) = %q, want empty", got)
	}
	if got := parseDatePtr(field(row, cols.settlementDate)); got != nil {
		t.Errorf("settlement date = %v, want nil", got)
	}
}

// A row of the retirement export, read through the resolved columns. Before the
// fix these values came from whatever happened to sit at the brokerage layout's
// positions - and the row was dropped outright for being too short.
func TestRetirementExportRowReadsThroughColumns(t *testing.T) {
	cols, err := resolveTxnColumns(retirementTxnHeader)
	if err != nil {
		t.Fatalf("resolveTxnColumns() error = %v", err)
	}
	row := []string{
		"08/11/2026", "YOU SOLD CHEVRON CORP NEW COM (CVX) (Cash)", "CVX",
		"CHEVRON CORP NEW COM", "Cash", "196.15", "-100", "", "0.41", "",
		"19614.09", "466868.66", "08/12/2026",
	}

	if got := field(row, cols.symbol); got != "CVX" {
		t.Errorf("symbol = %q, want CVX", got)
	}
	if got := parseDollarPtr(field(row, cols.quantity)); got == nil || *got != -100 {
		t.Errorf("quantity = %v, want -100", got)
	}
	if got := parseDollarPtr(field(row, cols.price)); got == nil || *got != 196.15 {
		t.Errorf("price = %v, want 196.15", got)
	}
	if got := parseDollar(field(row, cols.amount)); got != 19614.09 {
		t.Errorf("amount = %v, want 19614.09", got)
	}
	if got := parseDollar(field(row, cols.fees)); got != 0.41 {
		t.Errorf("fees = %v, want 0.41", got)
	}
	if got := parseDollar(field(row, cols.commission)); got != 0 {
		t.Errorf("commission = %v, want 0", got)
	}
	settled := parseDatePtr(field(row, cols.settlementDate))
	if settled == nil || settled.Format(fidelityDateLayout) != "08/12/2026" {
		t.Errorf("settlement date = %v, want 08/12/2026", settled)
	}
}

func TestNormalizeColumn(t *testing.T) {
	tests := map[string]string{
		"Amount ($)":           "amount",
		"  Run Date  ":         "run date",
		"\ufeffAccount number": "account number",
		"Settlement Date":      "settlement date",
	}
	for in, want := range tests {
		if got := normalizeColumn(in); got != want {
			t.Errorf("normalizeColumn(%q) = %q, want %q", in, got, want)
		}
	}
}
