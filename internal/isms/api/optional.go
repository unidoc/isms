package api

import (
	"bytes"
	"encoding/json"
)

// Optional is an update-request field that tells apart a key that is absent
// (leave the stored value alone), an explicit null (clear it), and a value
// (set it). A **T cannot do this: encoding/json decodes null into a **T by
// setting the outer pointer to nil, the same as an absent key (#381).
//
// Request structs use Optional[T] by value, so an absent key leaves the zero
// value with Set == false.
type Optional[T any] struct {
	// Set is true when the key was present in the request body.
	Set bool
	// Value is nil when the body said null (or an empty date string).
	Value *T
}

// UnmarshalJSON runs only for keys that are present in the body. An empty
// date (db.Epoch decoded from "") is normalised to nil so that "" keeps
// clearing a date, as it did before Optional existed, and so that no zero time
// ends up in a model struct. Numbers and booleans do not implement IsZero, so
// a real 0 or false is kept as a value.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	o.Value = nil
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	v := new(T)
	if err := json.Unmarshal(b, v); err != nil {
		return err
	}
	if z, ok := any(v).(interface{ IsZero() bool }); ok && z.IsZero() {
		return nil
	}
	o.Value = v
	return nil
}

// Ptr returns the double pointer form that the db update helpers take: nil
// when the key was absent, otherwise a pointer to Value (which is itself nil
// when the request cleared the field).
func (o Optional[T]) Ptr() **T {
	if !o.Set {
		return nil
	}
	return &o.Value
}
