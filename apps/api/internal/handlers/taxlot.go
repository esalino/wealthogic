package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"github.com/eriksalino/wealthogic/api/internal/portfolio"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Tax lots are derived from opening transactions (see models.TaxLot). These
// handlers read them, and add or edit a lot by way of the buy behind it.

type TaxLotHandler interface {
	GetTaxLots(c *gin.Context)
	CreateTaxLot(c *gin.Context)
	UpdateTaxLot(c *gin.Context)
}

type taxLotHandler struct {
	db *gorm.DB
}

func NewTaxLotHandler(db *gorm.DB) TaxLotHandler {
	return &taxLotHandler{db: db}
}

// taxLotView is a tax lot as the UI shows it: the lot itself, the trade that
// opened it, and what the open part of it is currently worth.
//
// The lot economics are computed here rather than in the client because they
// need things the lot row alone doesn't carry: the contract multiplier that
// scales an option's per-share price, and the holding's last price - the one
// place a current price lives.
//
// Two sets of figures, because a split resizes the lot but not the trade:
// Quantity, RemainingQuantity and AdjustedPrice are in today's shares, while
// PurchaseQuantity and PurchasePrice are the trade as it happened, which is
// what editing the lot edits. SplitFactor is today's shares per traded share,
// 1 when no split has intervened.
type taxLotView struct {
	ID                   uuid.UUID  `json:"id"`
	OpeningTransactionID uuid.UUID  `json:"opening_transaction_id"`
	AssetType            string     `json:"asset_type"`
	Symbol               string     `json:"symbol"`
	AssetDescription     string     `json:"asset_description"`
	PurchaseDate         time.Time  `json:"purchase_date"`
	PurchaseQuantity     float64    `json:"purchase_quantity"`
	PurchasePrice        float64    `json:"purchase_price"`
	Quantity             float64    `json:"quantity"`
	RemainingQuantity    float64    `json:"remaining_quantity"`
	AdjustedPrice        float64    `json:"adjusted_price"`
	SplitFactor          float64    `json:"split_factor"`
	HoldingID            *uuid.UUID `json:"holding_id"`
	AccountID            uuid.UUID  `json:"account_id"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`

	// Direction is "long" or "short"; a written option's lot took premium in
	// rather than paying it out, so its figures carry the opposite sign.
	Direction          string  `json:"direction"`
	ContractMultiplier float64 `json:"contract_multiplier"`
	LastPrice          float64 `json:"last_price"`

	// CostBasis is the open part of the lot at its opening price, commission
	// and fees included - negative for a short lot, where the premium came in.
	CostBasis float64 `json:"cost_basis"`

	// RealizedGains is what has been realized from the closed part of the lot.
	RealizedGains float64 `json:"realized_gains"`

	// MarketValue and the unrealized figures are nil when the holding has no
	// price: an unpriced lot has an unknown value, not a value of zero, and
	// reporting zero would show the whole basis as a loss.
	MarketValue           *float64 `json:"market_value"`
	GainUnrealizedAmount  *float64 `json:"gain_unrealized_amount"`
	GainUnrealizedPercent *float64 `json:"gain_unrealized_percent"`
} // @name TaxLot

func derefString(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}

func derefFloat(p *float64) float64 {
	if p != nil {
		return *p
	}
	return 0
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// lotView projects a lot, the trade that opened it, and its holding into the
// shape the UI expects. holding is nil when it can't be resolved - in which
// case there is no price and the value figures stay unknown.
func lotView(lot models.TaxLot, open models.Transaction, holding *models.Holding) taxLotView {
	multiplier, lastPrice := 1.0, 0.0
	if holding != nil {
		multiplier = holding.Multiplier()
		lastPrice = holding.LastPrice
	}

	traded := absFloat(derefFloat(open.Quantity))
	splitFactor := 1.0
	if traded != 0 {
		splitFactor = lot.Quantity / traded
	}

	basis := portfolio.OpenBasis(lot)
	var marketValue, gainAmount, gainPercent *float64
	if lastPrice != 0 {
		mv := lot.RemainingQuantity * lastPrice * multiplier
		if lot.Direction == models.DirectionShort {
			// The premium is money taken in, and closing the lot costs money -
			// so both sides sit on the opposite side of zero from a long lot,
			// and the gain is still value minus basis.
			basis, mv = -basis, -mv
		}
		gain := mv - basis
		marketValue, gainAmount = &mv, &gain
		if basis != 0 {
			pct := gain / absFloat(basis) * 100
			gainPercent = &pct
		}
	} else if lot.Direction == models.DirectionShort {
		basis = -basis
	}

	holdingID := lot.HoldingID
	return taxLotView{
		ID:                    lot.ID,
		OpeningTransactionID:  lot.OpeningTransactionID,
		RealizedGains:         lot.RealizedGains,
		Direction:             lot.Direction,
		ContractMultiplier:    multiplier,
		LastPrice:             lastPrice,
		CostBasis:             basis,
		MarketValue:           marketValue,
		GainUnrealizedAmount:  gainAmount,
		GainUnrealizedPercent: gainPercent,
		AssetType:             derefString(open.AssetType),
		Symbol:                open.Symbol,
		AssetDescription:      derefString(open.AssetDescription),
		PurchaseDate:          lot.AcquiredDate,
		PurchaseQuantity:      traded,
		PurchasePrice:         derefFloat(open.Price),
		Quantity:              lot.Quantity,
		RemainingQuantity:     lot.RemainingQuantity,
		AdjustedPrice:         derefFloat(open.Price) / splitFactor,
		SplitFactor:           splitFactor,
		HoldingID:             &holdingID,
		AccountID:             lot.AccountID,
		CreatedAt:             lot.CreatedAt,
		UpdatedAt:             lot.UpdatedAt,
	}
}

// lotViews resolves a page of lots against their opening trades and holdings in
// one query apiece, rather than one per lot.
func lotViews(db *gorm.DB, lots []models.TaxLot) ([]taxLotView, error) {
	txnIDs := make([]uuid.UUID, 0, len(lots))
	holdingIDs := make([]uuid.UUID, 0, len(lots))
	seen := map[uuid.UUID]bool{}
	for _, l := range lots {
		txnIDs = append(txnIDs, l.OpeningTransactionID)
		if !seen[l.HoldingID] {
			seen[l.HoldingID] = true
			holdingIDs = append(holdingIDs, l.HoldingID)
		}
	}

	txns := map[uuid.UUID]models.Transaction{}
	holdings := map[uuid.UUID]*models.Holding{}
	if len(lots) > 0 {
		var foundTxns []models.Transaction
		if err := db.Where("id IN ?", txnIDs).Find(&foundTxns).Error; err != nil {
			return nil, err
		}
		for _, t := range foundTxns {
			txns[t.ID] = t
		}
		var foundHoldings []models.Holding
		if err := db.Where("id IN ?", holdingIDs).Find(&foundHoldings).Error; err != nil {
			return nil, err
		}
		for i := range foundHoldings {
			holdings[foundHoldings[i].ID] = &foundHoldings[i]
		}
	}

	views := make([]taxLotView, len(lots))
	for i, l := range lots {
		views[i] = lotView(l, txns[l.OpeningTransactionID], holdings[l.HoldingID])
	}
	return views, nil
}

// lotViewForTransaction loads the lot an opening trade formed and projects it.
func lotViewForTransaction(db *gorm.DB, transactionID uuid.UUID) (taxLotView, error) {
	var lot models.TaxLot
	if err := db.Where("opening_transaction_id = ?", transactionID).First(&lot).Error; err != nil {
		return taxLotView{}, err
	}
	views, err := lotViews(db, []models.TaxLot{lot})
	if err != nil {
		return taxLotView{}, err
	}
	return views[0], nil
}

type paginatedTaxLots struct {
	Data     []taxLotView `json:"data"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
} // @name PaginatedTaxLots

// GetTaxLots godoc
// @Summary      List tax lots with pagination
// @Tags         tax-lots
// @Produce      json
// @Param        page        query     int     false  "Page number (default 1)"
// @Param        page_size   query     int     false  "Items per page (default 20, max 100)"
// @Param        holding_id  query     string  false  "Filter to a single holding"
// @Success      200         {object}  paginatedTaxLots
// @Failure      400         {object}  map[string]string
// @Failure      500         {object}  map[string]string
// @Router       /tax-lots [get]
func (h *taxLotHandler) GetTaxLots(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var holdingID *uuid.UUID
	if hid := c.Query("holding_id"); hid != "" {
		id, err := uuid.Parse(hid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid holding_id"})
			return
		}
		holdingID = &id
	}

	filter := func(q *gorm.DB) *gorm.DB {
		if holdingID != nil {
			q = q.Where("holding_id = ?", *holdingID)
		}
		return q
	}

	var total int64
	if err := filter(h.db.Model(&models.TaxLot{})).Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax lots"})
		return
	}

	var lots []models.TaxLot
	offset := (page - 1) * pageSize
	if err := filter(h.db).Order("acquired_date DESC").Order("opening_transaction_id DESC").
		Offset(offset).Limit(pageSize).Find(&lots).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax lots"})
		return
	}

	views, err := lotViews(h.db, lots)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax lots"})
		return
	}

	c.JSON(http.StatusOK, paginatedTaxLots{Data: views, Total: total, Page: page, PageSize: pageSize})
}

type createTaxLotRequest struct {
	HoldingID        uuid.UUID `json:"holding_id"`
	AccountID        uuid.UUID `json:"account_id"`
	PurchaseDate     string    `json:"purchase_date" binding:"required"`
	PurchaseQuantity float64   `json:"purchase_quantity" binding:"required"`
	PurchasePrice    float64   `json:"purchase_price" binding:"required"`
	Commission       float64   `json:"commission"`
	Fees             float64   `json:"fees"`
} // @name CreateTaxLotRequest

// taxLotWithTransaction is returned when a lot is added: the lot and the buy
// transaction recorded to open it.
type taxLotWithTransaction struct {
	TaxLot      taxLotView         `json:"tax_lot"`
	Transaction models.Transaction `json:"transaction"`
} // @name TaxLotWithTransaction

// CreateTaxLot godoc
// @Summary      Add a tax lot
// @Description  Records a buy against a holding, which opens the tax lot.
// @Tags         tax-lots
// @Accept       json
// @Produce      json
// @Param        tax_lot  body      createTaxLotRequest  true  "Tax lot payload"
// @Success      201      {object}  taxLotWithTransaction
// @Failure      400      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /tax-lots [post]
func (h *taxLotHandler) CreateTaxLot(c *gin.Context) {
	var req createTaxLotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.HoldingID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "holding_id is required"})
		return
	}
	if req.AccountID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}

	purchaseDate, err := parseInputDate(req.PurchaseDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "purchase_date must be YYYY-MM-DD or RFC3339"})
		return
	}

	var holding models.Holding
	if err := h.db.First(&holding, "id = ?", req.HoldingID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "holding not found"})
		return
	}

	var account models.Account
	if err := h.db.First(&account, "id = ?", req.AccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not found"})
		return
	}

	assetType := holding.AssetType
	description := holding.Description
	qty := req.PurchaseQuantity
	price := req.PurchasePrice
	amount := -(qty*price + req.Commission + req.Fees)

	txn := models.Transaction{
		AccountID:        req.AccountID,
		HoldingID:        &holding.ID,
		AssetType:        &assetType,
		Symbol:           holding.Symbol,
		AssetDescription: &description,
		Action:           "Buy",
		Effect:           models.EffectOpen,
		Direction:        models.DirectionLong,
		Date:             purchaseDate,
		Quantity:         &qty,
		Price:            &price,
		Amount:           amount,
		Commission:       req.Commission,
		Fees:             req.Fees,
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&txn).Error; err != nil {
			return err
		}
		if err := portfolio.OpenLot(tx, &txn); err != nil {
			return err
		}
		return portfolio.RecalcHolding(tx, &holding)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add tax lot"})
		return
	}

	view, err := lotViewForTransaction(h.db, txn.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tax lot"})
		return
	}
	c.JSON(http.StatusCreated, taxLotWithTransaction{TaxLot: view, Transaction: txn})
}

type updateTaxLotRequest struct {
	AccountID        uuid.UUID `json:"account_id"`
	PurchaseDate     string    `json:"purchase_date" binding:"required"`
	PurchaseQuantity float64   `json:"purchase_quantity" binding:"required"`
	PurchasePrice    float64   `json:"purchase_price" binding:"required"`
} // @name UpdateTaxLotRequest

// UpdateTaxLot godoc
// @Summary      Update a tax lot
// @Description  Updates the buy transaction behind the lot, in its as-traded shares, and rebuilds the holding's lots. Rejected if the lot has already been sold from.
// @Tags         tax-lots
// @Accept       json
// @Produce      json
// @Param        id       path      string               true  "Tax lot ID"
// @Param        tax_lot  body      updateTaxLotRequest  true  "Tax lot payload"
// @Success      200      {object}  taxLotView
// @Failure      400      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /tax-lots/{id} [patch]
func (h *taxLotHandler) UpdateTaxLot(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tax lot id"})
		return
	}

	var req updateTaxLotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.AccountID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required"})
		return
	}

	purchaseDate, err := parseInputDate(req.PurchaseDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "purchase_date must be YYYY-MM-DD or RFC3339"})
		return
	}

	var lot models.TaxLot
	if err := h.db.First(&lot, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tax lot not found"})
		return
	}
	var txn models.Transaction
	if err := h.db.Where("id = ? AND LOWER(action) = ?", lot.OpeningTransactionID, "buy").First(&txn).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only a lot opened by a buy can be edited here"})
		return
	}

	disposed, err := buyHasDisposals(h.db, txn.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tax lot"})
		return
	}
	if disposed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot edit a lot that has already been sold from"})
		return
	}

	var account models.Account
	if err := h.db.First(&account, "id = ?", req.AccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account not found"})
		return
	}

	qty := req.PurchaseQuantity
	price := req.PurchasePrice
	txn.AccountID = req.AccountID
	txn.Date = purchaseDate
	txn.Quantity = &qty
	txn.Price = &price
	txn.Amount = -(qty*price + txn.Commission + txn.Fees)

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&txn).Error; err != nil {
			return err
		}
		if txn.HoldingID != nil {
			return portfolio.RebuildHolding(tx, *txn.HoldingID, true)
		}
		return nil
	})
	if errors.Is(err, portfolio.ErrInsufficientShares) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not enough shares to cover existing sells"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tax lot"})
		return
	}

	view, err := lotViewForTransaction(h.db, txn.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tax lot"})
		return
	}
	c.JSON(http.StatusOK, view)
}
