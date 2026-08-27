package models

import (
	"database/sql/driver"
	"errors"
	"fmt"
)

// JSON is a minimal jsonb column type (raw bytes) so we avoid an extra
// dependency. Store/scan as-is; callers marshal/unmarshal their own shapes.
type JSON []byte

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
		return errors.New(fmt.Sprintf("JSON.Scan: unsupported type %T", src))
	}
	return nil
}
