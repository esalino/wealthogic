package uploads

import (
	"fmt"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Fidelity's exports overlap: a file named for 2026 can reach back a year or
// more to carry the opening lots of positions still held, and a combined
// 2024-2025 export is exactly its two single-year files concatenated. Importing
// two of them would otherwise record the same trade twice, inventing lots and
// the gains that come with them.
//
// So an import inserts only what the account doesn't already have. The match is
// on the fields the broker's row is made of, which is as close to an identity as
// the data provides - there is no transaction id in the export.

// signature identifies one imported row. Floats are rendered at fixed precision
// rather than compared directly, so a re-export that rounds differently in the
// last bits still matches.
type signature string

func txnSignature(date time.Time, action, effect, direction, symbol string, quantity *float64, amount float64) signature {
	q := "none"
	if quantity != nil {
		q = fmt.Sprintf("%.6f", *quantity)
	}
	return signature(fmt.Sprintf("%s|%s|%s|%s|%s|%s|%.4f",
		date.Format("2006-01-02"), action, effect, direction, symbol, q, amount))
}

func distSignature(date time.Time, category, symbol string, amount float64) signature {
	return signature(fmt.Sprintf("%s|%s|%s|%.4f", date.Format("2006-01-02"), category, symbol, amount))
}

// existingRows counts what the account already holds in the window a file
// covers, so an import can subtract it.
//
// Counts, not a set: two identical trades on one day are ordinary (a partial
// fill), and treating the second as a duplicate of the first would silently
// discard a real trade. An import inserts the excess of what the file has over
// what the account holds, so genuine repeats survive and re-imports don't stack.
type existingRows struct {
	counts map[signature]int
}

func loadExistingRows(tx *gorm.DB, accountID uuid.UUID, start, end *time.Time) (*existingRows, error) {
	e := &existingRows{counts: map[signature]int{}}
	if start == nil || end == nil {
		return e, nil
	}

	var txns []models.Transaction
	if err := tx.Where("account_id = ? AND date BETWEEN ? AND ?", accountID, *start, *end).
		Find(&txns).Error; err != nil {
		return nil, err
	}
	for _, t := range txns {
		e.counts[txnSignature(t.Date, t.Action, t.Effect, t.Direction, t.Symbol, t.Quantity, t.Amount)]++
	}

	var dists []models.Distribution
	if err := tx.Where("account_id = ? AND payment_date BETWEEN ? AND ?", accountID, *start, *end).
		Find(&dists).Error; err != nil {
		return nil, err
	}
	for _, d := range dists {
		e.counts[distSignature(d.PaymentDate, d.Category, d.Symbol, d.Amount)]++
	}

	return e, nil
}

// claim reports whether this row is already accounted for. Each call consumes
// one of the existing rows carrying that signature, so a file holding more
// copies than the account does still imports the surplus.
func (e *existingRows) claim(sig signature) bool {
	if e.counts[sig] <= 0 {
		return false
	}
	e.counts[sig]--
	return true
}
