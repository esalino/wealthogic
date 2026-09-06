package models

import (
	"time"

	"github.com/google/uuid"
)

// TaxLotTransaction ties a Transaction to a TaxLot: the buy that opened the lot,
// and each sell that disposed shares from it (a sell can span several lots).
// Quantity is the shares of the lot this transaction involves - acquired on a
// buy, disposed on a sell - which is what tax reporting resolves lot-by-lot
// gains from. It's a derived ledger, rebuilt when sells change, so no soft delete.
type TaxLotTransaction struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuidv7();primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	TaxLotID      uuid.UUID `gorm:"type:uuid;index" json:"tax_lot_id"`
	TransactionID uuid.UUID `gorm:"type:uuid;index" json:"transaction_id"`
	Quantity      float64   `json:"quantity"`
} // @name TaxLotTransaction
