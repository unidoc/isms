package api

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// TestUpdatePayloadFieldsMatchHandlers keeps updatePayloadFields in step with the
// keys each update apply handler actually reads from payload.Fields.
func TestUpdatePayloadFieldsMatchHandlers(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "api_suggestions.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing api_suggestions.go: %v", err)
	}
	decls := map[string]*ast.FuncDecl{}
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
			decls[fd.Name.Name] = fd
		}
	}
	seen := map[string]bool{}
	for key, fn := range applyRegistry {
		entity, kind, _ := strings.Cut(key, ":")
		if kind != "update" {
			continue
		}
		seen[entity] = true
		if _, typed := updateRequestTypes[entity]; typed {
			continue // covered by the request-type guard test (#200)
		}
		full := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
		name := full[strings.LastIndex(full, ".")+1:]
		fd := decls[name]
		if fd == nil {
			t.Errorf("%s: handler %s not found in api_suggestions.go", key, name)
			continue
		}
		read := map[string]bool{}
		ast.Inspect(fd, func(n ast.Node) bool {
			ix, ok := n.(*ast.IndexExpr)
			if !ok {
				return true
			}
			sel, ok := ix.X.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Fields" {
				return true
			}
			lit, ok := ix.Index.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if k, err := strconv.Unquote(lit.Value); err == nil {
				read[k] = true
			}
			return true
		})
		if entity == "audit_finding" {
			// Iterates the map instead of naming keys: status has its own branch,
			// UpdateAuditFindingFieldTx allows title and description.
			read = map[string]bool{"title": true, "description": true, "status": true}
		}
		allowed := updatePayloadFields[entity]
		if allowed == nil {
			t.Errorf("%s has an update handler but no updatePayloadFields entry", entity)
			continue
		}
		for k := range read {
			if _, ok := allowed[k]; !ok {
				t.Errorf("%s reads fields[%q] but updatePayloadFields[%q] does not list it", name, k, entity)
			}
		}
		for k := range allowed {
			if !read[k] {
				t.Errorf("updatePayloadFields[%q] lists %q but %s never reads it", entity, k, name)
			}
		}
	}
	for entity := range updatePayloadFields {
		if !seen[entity] {
			t.Errorf("updatePayloadFields has %q but there is no %s:update handler", entity, entity)
		}
	}
}

func TestCreatePayloadValidatorsCoverRegistry(t *testing.T) {
	for key := range applyRegistry {
		entity, kind, _ := strings.Cut(key, ":")
		if kind == "create" && createPayloadValidators[entity] == nil {
			t.Errorf("%s has a create handler but no createPayloadValidators entry", entity)
		}
	}
}

func assert400(t *testing.T, err error, wantInMsg ...string) {
	t.Helper()
	var he *echo.HTTPError
	if !errors.As(err, &he) || he.Code != http.StatusBadRequest {
		t.Fatalf("err = %v, want HTTP 400", err)
	}
	msg, _ := he.Message.(string)
	for _, w := range wantInMsg {
		if !strings.Contains(msg, w) {
			t.Errorf("message %q does not mention %q", msg, w)
		}
	}
}

func TestValidateUpdatePayload(t *testing.T) {
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }

	// Free-text suggestions: allowed at create/edit, refused at apply.
	for _, p := range []string{``, `null`, `{}`, `{"fields":{}}`, `{"fields":null}`} {
		if err := validateUpdatePayload("incident", raw(p), false); err != nil {
			t.Errorf("create-time %q: %v, want nil (free-text)", p, err)
		}
		assert400(t, validateUpdatePayload("incident", raw(p), true), "nothing to apply")
	}
	// #298: values at the top level.
	assert400(t, validateUpdatePayload("incident", raw(`{"status":"resolved"}`), false), `"fields"`, "status")
	assert400(t, validateUpdatePayload("incident", raw(`{"status":"resolved"}`), true), "nothing to apply", "status")
	assert400(t, validateUpdatePayload("incident", raw(`{"fields":{"status":"resolved"},"title":"x"}`), false), "title")
	// #200: unknown field, named with the supported list.
	assert400(t, validateUpdatePayload("incident", raw(`{"fields":{"bogus":"x"}}`), true), "bogus", "supported", "root_cause")
	// #200: wrong type instead of a silent skip.
	assert400(t, validateUpdatePayload("incident", raw(`{"fields":{"affects_c":"yes"}}`), true), "affects_c", "invalid field value")
	assert400(t, validateUpdatePayload("asset", raw(`{"fields":{"notes":3}}`), true), "notes", "invalid field value")
	assert400(t, validateUpdatePayload("asset", raw(`{"fields":{"notes":null}}`), true), "notes", "cannot be null")
	// #200: asset update accepts every field PUT does; null clears only Optional fields.
	assert400(t, validateUpdatePayload("asset", raw(`{"fields":{"confidentiality":"high"}}`), true), "confidentiality")
	assert400(t, validateUpdatePayload("asset", raw(`{"fields":{"bogus":1}}`), true), "bogus", "supported")
	assert400(t, validateUpdatePayload("system", raw(`{"fields":{"rpo_hours":"x"}}`), true), "rpo_hours")
	assert400(t, validateUpdatePayload("system", raw(`{"fields":{"name":null}}`), true), "cannot be null")
	assert400(t, validateUpdatePayload("supplier", raw(`{"fields":{"data_access":"yes"}}`), true), "data_access")
	assert400(t, validateUpdatePayload("risk", raw(`{"fields":{"custom_fields":null}}`), true), "custom_fields", "cannot be null")
	assert400(t, validateUpdatePayload("legal_requirement", raw(`{"fields":{"completion":"x"}}`), true), "completion")
	assert400(t, validateUpdatePayload("objective", raw(`{"fields":{"status":null}}`), true), "cannot be null")
	assert400(t, validateUpdatePayload("corrective_action", raw(`{"fields":{"notes":null}}`), true), "cannot be null")
	// Not JSON objects.
	assert400(t, validateUpdatePayload("asset", raw(`[1]`), false))
	assert400(t, validateUpdatePayload("asset", raw(`{"fields":[1]}`), false))
	// Valid.
	for _, tc := range []struct{ entity, payload string }{
		{"incident", `{"fields":{"status":"resolved","affects_c":true}}`},
		{"risk", `{"fields":{"notes":"n"}}`},
		{"asset", `{"fields":{"description":"d","confidentiality":3,"next_review":1893456000}}`},
		{"asset", `{"fields":{"confidentiality":null}}`},
		{"corrective_action", `{"fields":{"title":"t","due_date":null,"external_id":"x"}}`},
		{"incident", `{"fields":{"title":"t","data_breach":true,"authority_notified_at":null}}`},
		{"objective", `{"fields":{"target_operator":"lte","target_value":null,"checkin_cycle":4}}`},
		{"legal_requirement", `{"fields":{"url":"u","completion":50,"target_impact":null}}`},
		{"risk", `{"fields":{"current_likelihood":4,"treatment_due_date":null,"custom_fields":{}}}`},
		{"supplier", `{"fields":{"data_access":true,"contract_expiry":null}}`},
		{"system", `{"fields":{"rpo_hours":4,"supplier_id":null,"next_review":1893456000}}`},
		{"audit_finding", `{"fields":{"description":"d","status":"closed"}}`},
	} {
		if err := validateUpdatePayload(tc.entity, raw(tc.payload), true); err != nil {
			t.Errorf("%s %s: %v, want nil", tc.entity, tc.payload, err)
		}
	}
}

func TestValidateSuggestionPayloadCreate(t *testing.T) {
	raw := func(s string) json.RawMessage { return json.RawMessage(s) }
	// #200: the objective operator that used to be dropped and defaulted to gte.
	assert400(t, validateSuggestionPayload("objective", "create", raw(`{"title":"MTTP","bogus":"lte"}`)), "bogus")
	assert400(t, validateSuggestionPayload("corrective_action", "create", raw(`{"title":"c","bogus":1}`)), "bogus")
	assert400(t, validateSuggestionPayload("incident", "create", raw(`{"title":"i","summary":"s"}`)), "summary")
	assert400(t, validateSuggestionPayload("program", "create", raw(`{"key":"A","bogus":1}`)), "bogus")
	assert400(t, validateSuggestionPayload("asset", "create", raw(`{"name":"a","bogus":1}`)), "bogus")
	assert400(t, validateSuggestionPayload("system", "create", raw(`{"name":"a","bogus":1}`)), "bogus")
	assert400(t, validateSuggestionPayload("supplier", "create", raw(`{"name":"a","bogus":1}`)), "bogus")
	assert400(t, validateSuggestionPayload("risk", "create", raw(`{"title":"a","bogus":1}`)), "bogus")
	assert400(t, validateSuggestionPayload("legal_requirement", "create", raw(`{"title":"a","bogus":1}`)), "bogus")
	// Wrong type.
	assert400(t, validateSuggestionPayload("risk", "create", raw(`{"title":"r","current_likelihood":"high"}`)), "current_likelihood")
	// Valid, including an empty payload.
	for _, tc := range []struct{ entity, payload string }{
		{"risk", `{"title":"r","description":"d","category":"technology"}`},
		{"incident", `{}`},
		{"incident", ``},
		{"asset", `{"name":"a","confidentiality":3,"primary_location":"dc1","references":[{"type":"risk","id":"RISK-1"}]}`},
		{"asset", `{"title":"only a title"}`},
		{"corrective_action", `{"title":"c","severity":"major_nc","due_date":1893456000,"status":"todo"}`},
		{"incident", `{"title":"i","data_breach":true,"gdpr_role":"controller","detected_at":1767225600}`},
		{"objective", `{"title":"MTTP","target_operator":"lte","status":"active","checkin_cycle":4,"source":"probe","program_key":"AAA"}`},
		{"program", `{"key":"SEC","title":"Security","notes":"n"}`},
		{"legal_requirement", `{"title":"l","url":"u","completion":10,"treatment":"accept"}`},
		{"risk", `{"title":"r","target_likelihood":2,"owner":"a@b.c","next_review":1893456000}`},
		{"supplier", `{"name":"Acme","data_access":true,"contact":"c","next_review":1893456000}`},
		{"system", `{"name":"s","rpo_hours":4,"supplier_id":1,"references":[]}`},
		// Stored before #298 with the rationale copied in: must still decode.
		{"supplier", `{"name":"Acme","description":"copied rationale"}`},
	} {
		if err := validateSuggestionPayload(tc.entity, "create", raw(tc.payload)); err != nil {
			t.Errorf("%s create %q: %v, want nil", tc.entity, tc.payload, err)
		}
	}
	// Other suggestion types are not checked here.
	if err := validateSuggestionPayload("incident", "link", raw(`{"links":[{"type":"risk","id":"RISK-1"}]}`)); err != nil {
		t.Errorf("link: %v, want nil", err)
	}
}

func TestDecodeUpdateFields(t *testing.T) {
	type req struct {
		A *string       `json:"a"`
		B Optional[int] `json:"b"`
		C *bool         `json:"c"`
	}
	dec := func(payload string) (req, error) {
		var r req
		err := decodeUpdateFields("thing", json.RawMessage(payload), &r)
		return r, err
	}
	_, err := dec(`{"fields":{"zzz":1}}`)
	assert400(t, err, "zzz", "supported", "a, b, c")
	_, err = dec(`{"fields":{"a":null}}`)
	assert400(t, err, "cannot be null")
	r, err := dec(`{"fields":{"b":null}}`)
	if err != nil || !r.B.Set || r.B.Value != nil {
		t.Errorf("b:null => %+v, %v; want Set with nil Value", r, err)
	}
	_, err = dec(`{"fields":{"a":3}}`)
	assert400(t, err, "invalid field value")
	r, err = dec(`{"fields":{"c":true}}`)
	if err != nil || r.C == nil || !*r.C {
		t.Errorf("c:true => %+v, %v", r, err)
	}
}
