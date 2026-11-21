// Package util provides small helpers shared across packages.
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// SHA256Hex returns the lowercase hex SHA-256 digest of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// FirstNonEmpty returns the first value whose trimmed form is non-empty,
// stripping surrounding whitespace. It returns "" when all values are blank.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// ExpandHome expands a leading "~" or "~/" in path using home. When home is
// empty it falls back to the OS user home directory; if that is unavailable
// the path is returned unchanged. Paths without a leading "~" are returned
// as-is.
func ExpandHome(path, home string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

// TruncatePrefix keeps at most maxBytes bytes from the start of s without
// splitting a UTF-8 character, appending a marker with the dropped size.
func TruncatePrefix(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%s…[truncated %d bytes]", s[:cut], len(s)-cut)
}
