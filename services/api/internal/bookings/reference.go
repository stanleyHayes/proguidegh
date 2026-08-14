package bookings

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// referenceAlphabet excludes ambiguous characters (0/O, 1/I/L) so references
// survive phone calls and handwriting.
const referenceAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// newReference generates a human-readable unique booking reference
// ("PGH-XXXXX"). Uniqueness is enforced by the bookings_reference_key
// constraint; the caller retries on collision.
func newReference() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("bookings: reference entropy: %w", err)
	}
	out := make([]byte, 5)
	for i, b := range buf {
		out[i] = referenceAlphabet[int(b)%len(referenceAlphabet)]
	}
	return "PGH-" + string(out), nil
}

// errBadCursor rejects malformed pagination cursors.
var errBadCursor = errors.New("bookings: invalid cursor")

// encodeCursor builds the opaque keyset cursor for the tourist booking
// history: created_at + id, base64url-encoded. Cursor (keyset) pagination is
// used here rather than offset because booking history is append-mostly and
// grows unboundedly per tourist — spec §14 requires cursor pagination for
// high-volume history tables.
func encodeCursor(createdAt time.Time, id string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, "", errBadCursor
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok || id == "" {
		return time.Time{}, "", errBadCursor
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", errBadCursor
	}
	return t, id, nil
}
