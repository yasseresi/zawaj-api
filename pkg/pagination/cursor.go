// Package pagination provides opaque keyset (cursor) pagination helpers. Cursors
// encode a (created_at, id) position so paging is stable under inserts, unlike
// offset paging which drifts when rows are added.
package pagination

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Cursor marks a position in a newest-first stream: the (created_at, id) of the
// last item a client has seen.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// Encode returns an opaque, URL-safe cursor string.
func Encode(createdAt time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%d:%s", createdAt.UnixNano(), id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// Decode parses a cursor produced by Encode. An empty string yields a zero
// Cursor and ok=false (meaning "no cursor / first page"), not an error.
func Decode(s string) (Cursor, bool, error) {
	if s == "" {
		return Cursor{}, false, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, false, fmt.Errorf("cursor: bad encoding")
	}
	parts := strings.SplitN(string(b), ":", 2)
	if len(parts) != 2 {
		return Cursor{}, false, fmt.Errorf("cursor: malformed")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Cursor{}, false, fmt.Errorf("cursor: bad timestamp")
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return Cursor{}, false, fmt.Errorf("cursor: bad id")
	}
	return Cursor{CreatedAt: time.Unix(0, nanos), ID: id}, true, nil
}
