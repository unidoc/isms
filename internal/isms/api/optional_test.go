package api

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"isms.sh/internal/isms/db"
)

type optionalProbe struct {
	Date  Optional[db.Epoch] `json:"date"`
	Int   Optional[int]      `json:"int"`
	Int64 Optional[int64]    `json:"int64"`
	Float Optional[float64]  `json:"float"`
	Bool  Optional[bool]     `json:"bool"`
}

// TestOptionalDecode pins the three states of an update field: absent, null
// and value, for every element type used by the update requests (#381).
func TestOptionalDecode(t *testing.T) {
	epoch := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC).Unix()

	cases := []struct {
		name      string
		body      string
		isSet     func(p *optionalProbe) bool
		isNil     func(p *optionalProbe) bool
		valueIs   func(p *optionalProbe) bool
		wantSet   bool
		wantNil   bool
		wantValue bool
	}{
		{name: "date absent", body: `{}`,
			isSet: func(p *optionalProbe) bool { return p.Date.Set }},
		{name: "date null", body: `{"date":null}`, wantSet: true, wantNil: true,
			isSet: func(p *optionalProbe) bool { return p.Date.Set },
			isNil: func(p *optionalProbe) bool { return p.Date.Value == nil }},
		{name: "date empty string clears", body: `{"date":""}`, wantSet: true, wantNil: true,
			isSet: func(p *optionalProbe) bool { return p.Date.Set },
			isNil: func(p *optionalProbe) bool { return p.Date.Value == nil }},
		{name: "date epoch", body: `{"date":1803859200}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Date.Set },
			isNil:   func(p *optionalProbe) bool { return p.Date.Value == nil },
			valueIs: func(p *optionalProbe) bool { return p.Date.Value.Unix() == 1803859200 }},
		{name: "date string", body: `{"date":"2027-03-01"}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Date.Set },
			isNil:   func(p *optionalProbe) bool { return p.Date.Value == nil },
			valueIs: func(p *optionalProbe) bool { return p.Date.Value.Unix() == epoch }},
		{name: "int absent", body: `{}`,
			isSet: func(p *optionalProbe) bool { return p.Int.Set }},
		{name: "int null", body: `{"int":null}`, wantSet: true, wantNil: true,
			isSet: func(p *optionalProbe) bool { return p.Int.Set },
			isNil: func(p *optionalProbe) bool { return p.Int.Value == nil }},
		{name: "int value", body: `{"int":4}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Int.Set },
			isNil:   func(p *optionalProbe) bool { return p.Int.Value == nil },
			valueIs: func(p *optionalProbe) bool { return *p.Int.Value == 4 }},
		{name: "int zero is a value", body: `{"int":0}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Int.Set },
			isNil:   func(p *optionalProbe) bool { return p.Int.Value == nil },
			valueIs: func(p *optionalProbe) bool { return *p.Int.Value == 0 }},
		{name: "int64 absent", body: `{}`,
			isSet: func(p *optionalProbe) bool { return p.Int64.Set }},
		{name: "int64 null", body: `{"int64":null}`, wantSet: true, wantNil: true,
			isSet: func(p *optionalProbe) bool { return p.Int64.Set },
			isNil: func(p *optionalProbe) bool { return p.Int64.Value == nil }},
		{name: "int64 value", body: `{"int64":9000000000}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Int64.Set },
			isNil:   func(p *optionalProbe) bool { return p.Int64.Value == nil },
			valueIs: func(p *optionalProbe) bool { return *p.Int64.Value == 9000000000 }},
		{name: "int64 zero is a value", body: `{"int64":0}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Int64.Set },
			isNil:   func(p *optionalProbe) bool { return p.Int64.Value == nil },
			valueIs: func(p *optionalProbe) bool { return *p.Int64.Value == 0 }},
		{name: "float absent", body: `{}`,
			isSet: func(p *optionalProbe) bool { return p.Float.Set }},
		{name: "float null", body: `{"float":null}`, wantSet: true, wantNil: true,
			isSet: func(p *optionalProbe) bool { return p.Float.Set },
			isNil: func(p *optionalProbe) bool { return p.Float.Value == nil }},
		{name: "float value", body: `{"float":2.5}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Float.Set },
			isNil:   func(p *optionalProbe) bool { return p.Float.Value == nil },
			valueIs: func(p *optionalProbe) bool { return *p.Float.Value == 2.5 }},
		{name: "float zero is a value", body: `{"float":0}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Float.Set },
			isNil:   func(p *optionalProbe) bool { return p.Float.Value == nil },
			valueIs: func(p *optionalProbe) bool { return *p.Float.Value == 0 }},
		{name: "bool absent", body: `{}`,
			isSet: func(p *optionalProbe) bool { return p.Bool.Set }},
		{name: "bool null", body: `{"bool":null}`, wantSet: true, wantNil: true,
			isSet: func(p *optionalProbe) bool { return p.Bool.Set },
			isNil: func(p *optionalProbe) bool { return p.Bool.Value == nil }},
		{name: "bool true", body: `{"bool":true}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Bool.Set },
			isNil:   func(p *optionalProbe) bool { return p.Bool.Value == nil },
			valueIs: func(p *optionalProbe) bool { return *p.Bool.Value }},
		{name: "bool false is a value", body: `{"bool":false}`, wantSet: true, wantValue: true,
			isSet:   func(p *optionalProbe) bool { return p.Bool.Set },
			isNil:   func(p *optionalProbe) bool { return p.Bool.Value == nil },
			valueIs: func(p *optionalProbe) bool { return !*p.Bool.Value }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p optionalProbe
			if err := json.Unmarshal([]byte(tc.body), &p); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.body, err)
			}
			if got := tc.isSet(&p); got != tc.wantSet {
				t.Fatalf("Set = %v, want %v", got, tc.wantSet)
			}
			if !tc.wantSet {
				return
			}
			if got := tc.isNil(&p); got != tc.wantNil {
				t.Fatalf("Value == nil is %v, want %v", got, tc.wantNil)
			}
			if tc.wantValue && !tc.valueIs(&p) {
				t.Fatalf("Value does not match the body %s", tc.body)
			}
		})
	}
}

// TestOptionalDecodeWrongType checks that a wrongly typed value is a decode
// error rather than a silent clear.
func TestOptionalDecodeWrongType(t *testing.T) {
	for _, body := range []string{
		`{"int":"abc"}`,
		`{"int64":true}`,
		`{"float":"x"}`,
		`{"bool":"yes"}`,
		`{"date":"not a date"}`,
	} {
		var p optionalProbe
		if err := json.Unmarshal([]byte(body), &p); err == nil {
			t.Errorf("unmarshal %s: want an error, got none", body)
		}
	}
}

func TestOptionalPtr(t *testing.T) {
	var absent Optional[int]
	if absent.Ptr() != nil {
		t.Fatal("Ptr of an absent field must be nil")
	}

	cleared := Optional[int]{Set: true}
	pp := cleared.Ptr()
	if pp == nil || *pp != nil {
		t.Fatalf("Ptr of a cleared field must point at a nil value, got %v", pp)
	}

	n := 3
	set := Optional[int]{Set: true, Value: &n}
	pp = set.Ptr()
	if pp == nil || *pp == nil || **pp != 3 {
		t.Fatalf("Ptr of a set field must point at the value, got %v", pp)
	}
}

// TestUpdateRequestsHaveNoDoublePointers stops a **T from coming back in an
// update request. encoding/json decodes null into a **T as an outer nil, the
// same as an absent key, so such a field can never be cleared (#381). Use
// Optional[T] instead.
func TestUpdateRequestsHaveNoDoublePointers(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(assetUpdateRequest{}),
		reflect.TypeOf(supplierUpdateRequest{}),
		reflect.TypeOf(systemUpdateRequest{}),
		reflect.TypeOf(riskUpdateRequest{}),
		reflect.TypeOf(legalUpdateRequest{}),
		reflect.TypeOf(correctiveActionUpdateRequest{}),
		reflect.TypeOf(correctiveActionProgressRequest{}),
		reflect.TypeOf(incidentUpdateRequest{}),
		reflect.TypeOf(incidentProgressRequest{}),
		reflect.TypeOf(auditUpdateRequest{}),
		reflect.TypeOf(auditFindingUpdateRequest{}),
		reflect.TypeOf(taskUpdateRequest{}),
		reflect.TypeOf(changeUpdateRequest{}),
		reflect.TypeOf(objectiveUpdateRequest{}),
		reflect.TypeOf(checkinUpdateRequest{}),
	}
	for _, typ := range types {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.Type.Kind() == reflect.Ptr && f.Type.Elem().Kind() == reflect.Ptr {
				t.Errorf("%s.%s is %s; use Optional[T] so that null can clear the field", typ.Name(), f.Name, f.Type)
			}
		}
	}
}
