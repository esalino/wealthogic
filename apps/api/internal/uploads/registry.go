package uploads

import (
	"io"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Result summarizes what an upload handler did with a file.
type Result struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// Options carries request-level context a handler may need beyond the file
// itself, e.g. which account transactions should be tied to.
type Options struct {
	AccountID uuid.UUID
}

// FileHandler processes an uploaded file into database records.
type FileHandler interface {
	Process(db *gorm.DB, file io.Reader, opts Options) (*Result, error)
}

// Registry maps a (file type, account type) pair to the handler that knows how
// to parse that institution's file format.
type Registry struct {
	handlers map[string]FileHandler
}

func NewRegistry() *Registry {
	r := &Registry{handlers: map[string]FileHandler{}}
	r.register("holdings", "fidelity", &fidelityHoldingsHandler{})
	r.register("transactions", "fidelity", &fidelityTransactionsHandler{})
	return r
}

func (r *Registry) register(fileType, accountType string, h FileHandler) {
	r.handlers[key(fileType, accountType)] = h
}

func (r *Registry) Get(fileType, accountType string) (FileHandler, bool) {
	h, ok := r.handlers[key(fileType, accountType)]
	return h, ok
}

func key(fileType, accountType string) string {
	return strings.ToLower(strings.TrimSpace(fileType)) + ":" + strings.ToLower(strings.TrimSpace(accountType))
}
