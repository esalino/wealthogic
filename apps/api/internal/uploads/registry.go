package uploads

import (
	"io"
	"strings"

	"gorm.io/gorm"
)

// Result summarizes what an upload handler did with a file.
type Result struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// FileHandler processes an uploaded file into database records.
type FileHandler interface {
	Process(db *gorm.DB, file io.Reader) (*Result, error)
}

// Registry maps a (file type, account type) pair to the handler that knows how
// to parse that institution's file format.
type Registry struct {
	handlers map[string]FileHandler
}

func NewRegistry() *Registry {
	r := &Registry{handlers: map[string]FileHandler{}}
	r.register("holdings", "fidelity", &fidelityHoldingsHandler{})
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
