package db

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// Epoch is a time.Time that serializes to/from Unix epoch seconds in JSON.
// It embeds time.Time so pgx can scan TIMESTAMPTZ directly into it.
type Epoch struct {
	time.Time
}

// MarshalJSON outputs the time as Unix epoch seconds (integer).
// Zero time marshals as null.
func (e Epoch) MarshalJSON() ([]byte, error) {
	if e.IsZero() {
		return json.Marshal(nil)
	}
	return json.Marshal(e.Unix())
}

// UnmarshalJSON accepts epoch seconds (number), RFC3339 strings, date strings, or null.
func (e *Epoch) UnmarshalJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case float64:
		e.Time = time.Unix(int64(t), 0)
	case string:
		if t == "" {
			e.Time = time.Time{}
			return nil
		}
		// Try RFC3339 first
		parsed, err := time.Parse(time.RFC3339, t)
		if err == nil {
			e.Time = parsed
			return nil
		}
		// Try date-only (YYYY-MM-DD)
		parsed, err = time.Parse("2006-01-02", t)
		if err == nil {
			e.Time = parsed
			return nil
		}
		// Try Postgres-style timestamp
		parsed, err = time.Parse("2006-01-02T15:04:05", t)
		if err == nil {
			e.Time = parsed
			return nil
		}
		return fmt.Errorf("cannot parse time string %q", t)
	case nil:
		e.Time = time.Time{}
	}
	return nil
}

// Scan implements sql.Scanner so pgx can scan TIMESTAMPTZ in binary format.
func (e *Epoch) Scan(src interface{}) error {
	switch v := src.(type) {
	case time.Time:
		e.Time = v
		return nil
	case nil:
		e.Time = time.Time{}
		return nil
	default:
		return fmt.Errorf("cannot scan %T into Epoch", src)
	}
}

// ensure Epoch implements sql.Scanner
var _ sql.Scanner = (*Epoch)(nil)

// Value implements driver.Valuer so pgx can use Epoch as a parameter.
func (e Epoch) Value() (driver.Value, error) {
	if e.IsZero() {
		return nil, nil
	}
	return e.Time, nil
}

// NewEpoch wraps a time.Time as an Epoch.
func NewEpoch(t time.Time) Epoch {
	return Epoch{Time: t}
}

// EpochNow returns an Epoch set to the current time.
func EpochNow() Epoch {
	return Epoch{Time: time.Now()}
}
