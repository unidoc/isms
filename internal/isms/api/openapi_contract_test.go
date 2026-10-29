package api

// Contract test: the embedded OpenAPI spec must describe what the handlers
// really serialize. For every component listed in contractTable it builds the
// Go value the handler passes to c.JSON, fills every field by reflection,
// marshals it with encoding/json and compares the result with the schema, in
// both directions (keys) and by JSON type. A second pass marshals the zero
// value to check nullability and omitempty. See the rules below.
//
// Nullability rule (mirrors the EpochSeconds description in openapi.yaml):
//   - a key that is omitted when empty (omitempty) is not in "required" and its
//     type does not include null;
//   - a key that marshals to null at its zero value (nil pointer, nil slice or
//     map, zero db.Epoch) has a type that includes null;
//   - every other key is never null, so its type does not include null;
//   - neverNullKeys are exempt from the second pass: the database always sets
//     them (now() defaults), so their zero value is an artifact of the test and
//     the spec documents them as non-null.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"isms.sh/internal/isms/db"
)

type contractEntry struct {
	component string
	// typ is the Go type the handler serializes (pointer stripped).
	typ reflect.Type
	// handler documents where the value comes from: file:line and the
	// expression passed to c.JSON. A wrong mapping would pass green against
	// the wrong shape, so keep it traced to the real handler.
	handler string
	// requests are the request DTOs of the endpoints that take this component
	// as a request body. Used to check the readOnly convention.
	requests []reflect.Type
}

var contractTable = []contractEntry{
	{
		component: "Risk",
		typ:       reflect.TypeOf(db.Risk{}),
		// All four handlers return the db.Risk itself, never a wrapper:
		//   server.go:2259 handleGetRisk:    c.JSON(200, r)     r is *db.Risk (GetRisk)
		//   server.go:2450 handleAddRisk:    c.JSON(201, out)   out is *db.Risk (GetRisk)
		//   server.go:3036 handleUpdateRisk: c.JSON(200, after) after is *db.Risk (GetRisk)
		//   server.go:2368 handleListRisks:  c.JSON(200, {"data": risks, ...}) risks is
		//     []db.Risk (PaginatedRisks); the list wrapper is an inline schema in
		//     the /risks path, not part of the Risk component.
		handler:  "server.go:2259 handleGetRisk: c.JSON(200, r), r is *db.Risk",
		requests: []reflect.Type{reflect.TypeOf(riskCreateRequest{}), reflect.TypeOf(riskUpdateRequest{})},
	},
}

// noGoType lists components that are not responses of any handler, with the
// reason.
var noGoType = map[string]string{
	"EpochSeconds":         "primitive shared by response schemas (db.Epoch), no handler returns it alone",
	"EpochSecondsNullable": "primitive shared by response schemas (db.Epoch), no handler returns it alone",
}

// pendingPartB lists the components not yet checked against their Go type.
// The list shrinks as schemas are aligned; the goal is an empty list.
var pendingPartB = map[string]bool{
	"Error": true, "User": true, "UserWithRole": true, "Organization": true,
	"APIKey": true, "Document": true, "SearchResult": true,
	"DocumentVersion": true, "Asset": true, "Supplier": true, "System": true,
	"AccessReview": true, "LegalRequirement": true, "Incident": true,
	"CorrectiveAction": true, "Review": true, "Comment": true,
	"Approval": true, "Task": true, "ChangeRequest": true, "Suggestion": true,
	"ImplementationStatus": true, "Notification": true,
	"AuditProgramme": true, "Audit": true, "AuditItem": true,
	"AuditFinding": true, "Program": true, "Objective": true, "Checkin": true,
	"Evidence": true, "EntityReference": true, "ChangelogEntry": true,
	"OIDCProvider": true,
}

// neverNullKeys are JSON keys the database always fills; see the header.
var neverNullKeys = map[string]bool{"created_at": true, "updated_at": true}

type specSchemas map[string]any

func loadSpecSchemas(t *testing.T) specSchemas {
	t.Helper()
	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	if len(doc.Components.Schemas) == 0 {
		t.Fatal("openapi.yaml has no components/schemas")
	}
	return doc.Components.Schemas
}

// resolve follows $ref (within components/schemas) and overlays sibling
// keywords such as description and readOnly, which OpenAPI 3.1 allows.
func (s specSchemas) resolve(node any) (map[string]any, error) {
	m, ok := node.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema is %T, not a mapping", node)
	}
	ref, ok := m["$ref"].(string)
	if !ok {
		return m, nil
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("unsupported $ref %q", ref)
	}
	target, ok := s[strings.TrimPrefix(ref, prefix)]
	if !ok {
		return nil, fmt.Errorf("$ref %q does not resolve", ref)
	}
	base, err := s.resolve(target)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(base)+len(m))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range m {
		if k != "$ref" {
			merged[k] = v
		}
	}
	return merged, nil
}

func schemaTypes(m map[string]any) []string {
	switch v := m["type"].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func hasType(types []string, want string) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}

func allowsNull(m map[string]any) bool { return hasType(schemaTypes(m), "null") }

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if x == float64(int64(x)) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", v)
}

func isRequired(m map[string]any, key string) bool {
	list, _ := m["required"].([]any)
	for _, r := range list {
		if r == key {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// checkFull compares a fully populated response value with its schema.
func (s specSchemas) checkFull(path string, val any, node any, errs *[]string) {
	sch, err := s.resolve(node)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: %v", path, err))
		return
	}
	for _, kw := range []string{"allOf", "oneOf", "anyOf"} {
		if _, ok := sch[kw]; ok {
			*errs = append(*errs, fmt.Sprintf("%s: %s is not supported by the contract test", path, kw))
			return
		}
	}
	types := schemaTypes(sch)
	if len(types) == 0 {
		*errs = append(*errs, fmt.Sprintf("%s: schema has no type", path))
		return
	}
	jt := jsonType(val)
	if !hasType(types, jt) && !(jt == "integer" && hasType(types, "number")) {
		*errs = append(*errs, fmt.Sprintf("%s: spec type %s, emitted %s", path, strings.Join(types, "|"), jt))
		return
	}
	switch v := val.(type) {
	case map[string]any:
		props, _ := sch["properties"].(map[string]any)
		if props == nil {
			if ap, ok := sch["additionalProperties"].(map[string]any); ok {
				for _, k := range sortedKeys(v) {
					s.checkFull(path+"."+k, v[k], ap, errs)
				}
			}
			return
		}
		for _, k := range sortedKeys(props) {
			if _, ok := v[k]; !ok {
				*errs = append(*errs, fmt.Sprintf("%s: spec documents %q but the response has no such key", path, k))
			}
		}
		for _, k := range sortedKeys(v) {
			p, ok := props[k]
			if !ok {
				*errs = append(*errs, fmt.Sprintf("%s: response has %q but the spec does not document it", path, k))
				continue
			}
			s.checkFull(path+"."+k, v[k], p, errs)
		}
	case []any:
		if items, ok := sch["items"]; ok {
			for i, e := range v {
				s.checkFull(fmt.Sprintf("%s[%d]", path, i), e, items, errs)
			}
		}
	}
}

// checkZero compares the zero-value marshaling (full is the populated one,
// used to learn which keys exist) with the schema's nullability and required.
func (s specSchemas) checkZero(path string, zero, full map[string]any, node any, errs *[]string) {
	sch, err := s.resolve(node)
	if err != nil {
		return // already reported by checkFull
	}
	props, _ := sch["properties"].(map[string]any)
	for _, k := range sortedKeys(full) {
		pnode, ok := props[k]
		if !ok {
			continue // reported by checkFull
		}
		ps, err := s.resolve(pnode)
		if err != nil {
			continue
		}
		p := path + "." + k
		zv, present := zero[k]
		switch {
		case neverNullKeys[k]:
			if allowsNull(ps) {
				*errs = append(*errs, fmt.Sprintf("%s: the database always sets this key, spec type must not include null", p))
			}
		case !present:
			if allowsNull(ps) {
				*errs = append(*errs, fmt.Sprintf("%s: omitted when empty (omitempty), so the spec type must not include null", p))
			}
			if isRequired(sch, k) {
				*errs = append(*errs, fmt.Sprintf("%s: omitted when empty (omitempty), so it must not be in required", p))
			}
		case zv == nil:
			if !allowsNull(ps) {
				*errs = append(*errs, fmt.Sprintf("%s: emitted null at its zero value, spec type %s does not include null", p, strings.Join(schemaTypes(ps), "|")))
			}
		default:
			if allowsNull(ps) {
				*errs = append(*errs, fmt.Sprintf("%s: spec type includes null but the response never emits null for it", p))
			}
			zm, zok := zv.(map[string]any)
			fm, fok := full[k].(map[string]any)
			if zok && fok {
				s.checkZero(p, zm, fm, ps, errs)
			}
		}
	}
	for _, r := range func() []any { l, _ := sch["required"].([]any); return l }() {
		name, _ := r.(string)
		if _, ok := props[name]; !ok {
			*errs = append(*errs, fmt.Sprintf("%s: required lists %q which is not a property", path, name))
		}
	}
}

var (
	epochType   = reflect.TypeOf(db.Epoch{})
	timeType    = reflect.TypeOf(time.Time{})
	rawJSONType = reflect.TypeOf(json.RawMessage{})
)

// fill sets every exported field: pointers non-nil, slices and maps with one
// element, Epoch and time non-zero, json.RawMessage a valid JSON object.
func fill(v reflect.Value, depth int) {
	if depth > 6 {
		return
	}
	switch v.Type() {
	case epochType:
		v.Set(reflect.ValueOf(db.Epoch{Time: time.Unix(1700000000, 0).UTC()}))
		return
	case timeType:
		v.Set(reflect.ValueOf(time.Unix(1700000000, 0).UTC()))
		return
	case rawJSONType:
		v.SetBytes([]byte(`{"k":"v"}`))
		return
	}
	switch v.Kind() {
	case reflect.Ptr:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem(), depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				fill(v.Field(i), depth+1)
			}
		}
	case reflect.Slice:
		sl := reflect.MakeSlice(v.Type(), 1, 1)
		fill(sl.Index(0), depth+1)
		v.Set(sl)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		fill(key, depth+1)
		val := reflect.New(v.Type().Elem()).Elem()
		fill(val, depth+1)
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.Interface:
		v.Set(reflect.ValueOf("x"))
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	}
}

func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("response is not a JSON object: %v", err)
	}
	return m
}

// jsonKeys returns the JSON keys a request DTO accepts at the top level.
func jsonKeys(t reflect.Type, into map[string]bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			jsonKeys(f.Type, into)
			continue
		}
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		into[name] = true
	}
}

func TestOpenAPIContract(t *testing.T) {
	schemas := loadSpecSchemas(t)
	for _, e := range contractTable {
		t.Run(e.component, func(t *testing.T) {
			node, ok := schemas[e.component]
			if !ok {
				t.Fatalf("%s: in contractTable but not in components/schemas", e.component)
			}
			filled := reflect.New(e.typ)
			fill(filled.Elem(), 0)
			full := marshalToMap(t, filled.Interface())
			zero := marshalToMap(t, reflect.New(e.typ).Interface())

			var errs []string
			schemas.checkFull(e.component, full, node, &errs)
			schemas.checkZero(e.component, zero, full, node, &errs)

			// readOnly convention: a property is readOnly exactly when none
			// of the request DTOs accepts it.
			if len(e.requests) > 0 {
				accepted := map[string]bool{}
				for _, r := range e.requests {
					jsonKeys(r, accepted)
				}
				sch, err := schemas.resolve(node)
				if err == nil {
					props, _ := sch["properties"].(map[string]any)
					for _, k := range sortedKeys(props) {
						ps, err := schemas.resolve(props[k])
						if err != nil {
							continue
						}
						ro, _ := ps["readOnly"].(bool)
						switch {
						case ro && accepted[k]:
							errs = append(errs, fmt.Sprintf("%s.%s: marked readOnly but the request body accepts it", e.component, k))
						case !ro && !accepted[k]:
							errs = append(errs, fmt.Sprintf("%s.%s: not readOnly but the request body does not accept it", e.component, k))
						}
					}
				}
			}
			sort.Strings(errs)
			for _, msg := range errs {
				t.Error(msg)
			}
		})
	}
}

// Every component must be checked, deliberately exempt, or pending.
func TestOpenAPIComponentsAccountedFor(t *testing.T) {
	schemas := loadSpecSchemas(t)
	inTable := map[string]bool{}
	for _, e := range contractTable {
		inTable[e.component] = true
	}
	for _, name := range sortedKeys(schemas) {
		n := 0
		if inTable[name] {
			n++
		}
		if _, ok := noGoType[name]; ok {
			n++
		}
		if pendingPartB[name] {
			n++
		}
		switch n {
		case 0:
			t.Errorf("component %s is not checked: add it to contractTable (with its handler file:line), noGoType or pendingPartB", name)
		case 1:
		default:
			t.Errorf("component %s is listed in more than one of contractTable, noGoType and pendingPartB", name)
		}
	}
	for name := range noGoType {
		if _, ok := schemas[name]; !ok {
			t.Errorf("noGoType lists %s which is not in components/schemas", name)
		}
	}
	for name := range pendingPartB {
		if _, ok := schemas[name]; !ok {
			t.Errorf("pendingPartB lists %s which is not in components/schemas", name)
		}
	}
}

func TestOpenAPIEpochSchemas(t *testing.T) {
	schemas := loadSpecSchemas(t)
	for name, want := range map[string]string{"EpochSeconds": "integer", "EpochSecondsNullable": "integer|null"} {
		node, ok := schemas[name]
		if !ok {
			t.Errorf("%s is missing from components/schemas", name)
			continue
		}
		sch, err := schemas.resolve(node)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := strings.Join(schemaTypes(sch), "|"); got != want {
			t.Errorf("%s: spec type %s, want %s", name, got, want)
		}
		if sch["format"] != "int64" {
			t.Errorf("%s: format must be int64, got %v", name, sch["format"])
		}
	}
}
