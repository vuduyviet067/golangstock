package usecase

import (
	"strings"
	"unicode/utf8"
)

// maxCatalogNameLength is the longest wood type or supplier name, in
// characters (not bytes), as in the CHECK constraints of migration 000009.
const maxCatalogNameLength = 100

// normalizeCatalogName trims the name and reports whether it is valid: 1 to
// 100 characters after trimming.
func normalizeCatalogName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= maxCatalogNameLength
}
