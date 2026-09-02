package models

import (
	"database/sql/driver"
	"errors"
	"fmt"
)

// JSON is a minimal jsonb column type (raw bytes) so we avoid an extra
// dependency. Store/scan as-is; callers marshal/unmarshal their own shapes.
type JSON []byte

// MarshalJSON emits the stored bytes verbatim so a jsonb column surfaces as a
// real JSON value in responses (not a base64 string). Empty → null.
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

// UnmarshalJSON stores the raw JSON bytes as-is.
func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("models.JSON: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[:0], data...)
	return nil
}

// Value implements driver.Valuer.
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

// Scan implements sql.Scanner.
func (j *JSON) Scan(src any) error {
	if src == nil {
		*j = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = []byte(v)
	default:
		return fmt.Errorf("JSON.Scan: unsupported type %T", src)
	}
	return nil
}
