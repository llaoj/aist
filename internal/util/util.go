// Package util provides small helpers shared across packages.
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const localMinuteLayout = "2006-01-02 15:04"

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

// FormatLocalMinute formats a timestamp in the local timezone to minute precision.
func FormatLocalMinute(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format(localMinuteLayout)
}

// FormatThousands formats an integer with ASCII comma thousands separators.
func FormatThousands(n int) string {
	raw := strconv.Itoa(n)
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = raw[1:]
	}
	if len(raw) <= 3 {
		return sign + raw
	}
	var b strings.Builder
	b.Grow(len(sign) + len(raw) + (len(raw)-1)/3)
	b.WriteString(sign)
	lead := len(raw) % 3
	if lead > 0 {
		b.WriteString(raw[:lead])
	}
	for i := lead; i < len(raw); i += 3 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(raw[i : i+3])
	}
	return b.String()
}
