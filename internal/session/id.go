package session

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// SystemClock uses the wall clock.
type SystemClock struct{}

// Now returns the current local time in UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// TimestampIDGenerator creates sortable timestamp-prefixed identifiers.
type TimestampIDGenerator struct{}

// NewSessionID returns "ses_<utc timestamp>-<random>".
func (TimestampIDGenerator) NewSessionID(now time.Time) string {
	return "ses_" + now.UTC().Format("20060102T150405.000000000Z") + "-" + randomSuffix(4)
}

// NewMessageID returns "msg_<utc timestamp>-<random>".
func (TimestampIDGenerator) NewMessageID(now time.Time) string {
	return "msg_" + now.UTC().Format("20060102T150405.000000000Z") + "-" + randomSuffix(3)
}

// NewCompactionID returns "cmp_<utc timestamp>-<random>".
func (TimestampIDGenerator) NewCompactionID(now time.Time) string {
	return "cmp_" + now.UTC().Format("20060102T150405.000000000Z") + "-" + randomSuffix(3)
}

func randomSuffix(nBytes int) string {
	data := make([]byte, nBytes)
	if _, err := rand.Read(data); err != nil {
		return time.Now().UTC().Format("000000000")
	}
	return hex.EncodeToString(data)
}
