package uploads

import (
	"fmt"
	"strings"
	"time"

	"github.com/eriksalino/wealthogic/api/internal/models"
	"gorm.io/gorm"
)

// File types an import can be registered and recorded under. The registry keys
// on these and every Upload row stores one, so the two can't drift apart.
const (
	FileTypeHoldings     = "holdings"
	FileTypeTransactions = "transactions"
)

// baseFileName strips any directory part from an uploaded file's name.
//
// A browser sends a bare name, but it isn't the only client: a path reaches
// here from anything that uploads by filename, and history showing
// "../../Positions.csv" is reporting where the caller's shell happened to be
// rather than which file was imported. Both separators are stripped, since a
// Windows client's backslashes mean nothing to a unix server's filepath.
func baseFileName(name string) string {
	name = strings.TrimSpace(name)
	// A trailing separator would otherwise leave the name empty.
	trimmed := strings.TrimRight(name, `/\`)
	if trimmed == "" {
		return name
	}
	if i := strings.LastIndexAny(trimmed, `/\`); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}

// recordUpload logs the file itself, so every import appears in history -
// including a positions snapshot, which produces no transaction rows that could
// stand in for it.
//
// start and end bound the dates the file covers and are nil for a snapshot. It
// is called only once an import has recognized the file as its own, so a
// rejected upload leaves no row behind.
func recordUpload(tx *gorm.DB, fileType string, opts Options, start, end *time.Time) (*models.Upload, error) {
	upload := models.Upload{
		FileName:  baseFileName(opts.FileName),
		FileType:  fileType,
		AccountID: opts.AccountID,
		StartDate: start,
		EndDate:   end,
	}
	if err := tx.Create(&upload).Error; err != nil {
		return nil, fmt.Errorf("failed to create upload: %w", err)
	}
	return &upload, nil
}
