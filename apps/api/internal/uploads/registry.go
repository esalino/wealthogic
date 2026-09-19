package uploads

import (
	"io"
	"strings"

	"github.com/eriksalino/wealthogic/api/internal/marketdata"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Result summarizes what an upload handler did with a file. Duplicates are
// counted apart from Skipped: a skipped row was unusable, a duplicate was
// already imported, and only the latter is expected when files overlap.
type Result struct {
	Created    int `json:"created"`
	Updated    int `json:"updated"`
	Skipped    int `json:"skipped"`
	Duplicates int `json:"duplicates"`
} // @name UploadResult

// Options carries request-level context a handler may need beyond the file
// itself, e.g. which account transactions should be tied to and the name of the
// uploaded file.
type Options struct {
	AccountID uuid.UUID
	FileName  string

	// Enricher fills in reference data for holdings the import creates. Nil is
	// fine and means the detail is simply skipped - an import must not depend
	// on an external provider being reachable.
	Enricher *marketdata.Enricher
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
