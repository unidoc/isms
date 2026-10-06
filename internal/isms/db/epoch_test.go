package db

import (
	"encoding/json"
	"testing"
)

func TestEpochUnmarshalJSONStrictTypes(t *testing.T) {
	for _, in := range []string{`true`, `{}`, `[]`} {
		var e Epoch
		if err := json.Unmarshal([]byte(in), &e); err == nil {
			t.Errorf("%s: want error, got nil", in)
		}
	}
	for _, in := range []string{`1893456000`, `"2030-01-01"`, `""`, `null`} {
		var e Epoch
		if err := json.Unmarshal([]byte(in), &e); err != nil {
			t.Errorf("%s: unexpected error %v", in, err)
		}
	}
}
