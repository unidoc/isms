package api

// Contract test: the embedded OpenAPI spec must describe what the handlers
// really serialize. For every component listed in contractTable it builds the
// Go value the handler passes to c.JSON, fills every field by reflection,
// marshals it with encoding/json and compares the result with the schema, in
// both directions (keys) and by JSON type. A second pass marshals the zero
// value to check nullability and omitempty. See the rules below.
//
// readOnly and writeOnly follow the request DTOs listed per entry: a property
// is readOnly when no request accepts it (or the handler overwrites it), and
// writeOnly when a request accepts it but the response never emits it. A
// component that an operation takes as its request body must list its DTOs,
// and each DTO field must be able to decode the type the schema documents.
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

// contractShape is one Go value a handler passes to c.JSON for a component.
type contractShape struct {
	// typ is the Go type serialized (pointer stripped).
	typ reflect.Type
	// handler documents where the value comes from: file:line and the
	// expression passed to c.JSON. A wrong mapping would pass green against
	// the wrong shape, so keep it traced to the real handler.
	handler string
}

type contractEntry struct {
	component string
	// shapes holds one value per distinct handler shape (for example a list
	// handler that joins an extra column). Each is checked for undocumented
	// keys and wrong types; a documented key must appear in at least one shape,
	// and a key missing from some shapes must not be required.
	shapes []contractShape
	// requests are the request DTOs that accept this component's keys. A
	// component that some requestBody references MUST list them.
	requests []reflect.Type
	// serverSet lists keys the request type binds but the handler overwrites or
	// ignores (for example when the request body is the db struct itself); the
	// spec marks them readOnly.
	serverSet []string
	// requestOnly marks a component that no handler returns: it is the element
	// type of a request field, checked against that Go type.
	requestOnly bool
}

func shape(t reflect.Type, handler string) contractShape {
	return contractShape{typ: t, handler: handler}
}

var tReq = reflect.TypeOf

// contractTable maps each checked component to the Go values handlers
// serialize. Every handler below returns the db struct itself (never a
// wrapper, except list handlers which wrap the rows as {"data": rows, ...} in
// an inline schema outside the component), so one shape per component.
var contractTable = []contractEntry{
	{
		component: "Risk",
		// All handlers return the db.Risk itself, never a wrapper:
		//   server.go:2259 handleGetRisk:    c.JSON(200, r)     r is *db.Risk (GetRisk)
		//   server.go:2450 handleAddRisk:    c.JSON(201, out)   out is *db.Risk (GetRisk)
		//   server.go:3036 handleUpdateRisk: c.JSON(200, after) after is *db.Risk (GetRisk)
		//   server.go:2368 handleListRisks:  c.JSON(200, {"data": risks, ...}) risks is
		//     []db.Risk (PaginatedRisks); the list wrapper is an inline schema in
		//     the /risks path, not part of the Risk component.
		shapes:   []contractShape{shape(tReq(db.Risk{}), "server.go:2259 handleGetRisk: c.JSON(200, r), r is *db.Risk")},
		requests: []reflect.Type{tReq(riskCreateRequest{}), tReq(riskUpdateRequest{})},
	},
	{
		component: "ReferenceInput",
		// Not returned by any handler: element type of the "references" field
		// of the create requests (api_references.go:17).
		shapes:      []contractShape{shape(tReq(ReferenceInput{}), "api_references.go:17 ReferenceInput, element of the references field of create requests")},
		requestOnly: true,
	},
	{
		component: "Asset",
		//   server.go:2031 handleGetAsset:    c.JSON(200, a)       a is *db.Asset (GetAsset)
		//   server.go:2083 handleAddAsset:    c.JSON(201, out)     out is *db.Asset (GetAsset); 2085 falls back to a (db.Asset)
		//   server.go:2819 handleUpdateAsset: c.JSON(200, after)   after is *db.Asset (GetAsset); 2821 falls back to updated (*db.Asset)
		//   server.go:2002 handleListAssets:  {"data": assets} assets is []db.Asset (PaginatedAssets)
		shapes:   []contractShape{shape(tReq(db.Asset{}), "server.go:2031 handleGetAsset: c.JSON(200, a), a is *db.Asset")},
		requests: []reflect.Type{tReq(assetCreateRequest{}), tReq(assetUpdateRequest{})},
	},
	{
		component: "System",
		//   server.go:2171 handleGetSystem:    c.JSON(200, sys)    sys is *db.System (GetSystem)
		//   server.go:2244 handleCreateSystem: c.JSON(201, sys)    sys is db.System (re-fetched via GetSystem)
		//   server.go:3168 handleUpdateSystem: c.JSON(200, after)  after is *db.System (GetSystem); 3170 falls back to updated
		//   server.go:2142 handleListSystems:  {"data": systems} systems is []db.System (PaginatedSystems)
		shapes:   []contractShape{shape(tReq(db.System{}), "server.go:2171 handleGetSystem: c.JSON(200, sys), sys is *db.System")},
		requests: []reflect.Type{tReq(systemCreateRequest{}), tReq(systemUpdateRequest{})},
	},
	{
		component: "AccessReview",
		//   server.go:3256 handleCreateAccessReview: c.JSON(201, ar) ar is db.AccessReview (also the bound request body)
		//   server.go:3217 handleListAccessReviews:  {"data": reviews} reviews is []db.AccessReview (ListAccessReviews)
		// The request body is the db struct itself, so every key binds; the
		// handler sets system_id, CreateAccessReview sets organization_id, id
		// and created_at.
		shapes:    []contractShape{shape(tReq(db.AccessReview{}), "server.go:3256 handleCreateAccessReview: c.JSON(201, ar), ar is db.AccessReview")},
		requests:  []reflect.Type{tReq(db.AccessReview{})},
		serverSet: []string{"id", "organization_id", "system_id", "created_at"},
	},
	{
		component: "Supplier",
		//   server.go:2654 handleGetSupplier:    c.JSON(200, sup)    sup is *db.Supplier (GetSupplier)
		//   server.go:2712 handleAddSupplier:    c.JSON(201, sup)    sup is db.Supplier (re-fetched via GetSupplier)
		//   server.go:3396 handleUpdateSupplier: c.JSON(200, after)  after is *db.Supplier (GetSupplier); 3398 falls back to updated
		//   server.go:2625 handleListSuppliers:  {"data": suppliers} suppliers is []db.Supplier (PaginatedSuppliers)
		shapes:   []contractShape{shape(tReq(db.Supplier{}), "server.go:2654 handleGetSupplier: c.JSON(200, sup), sup is *db.Supplier")},
		requests: []reflect.Type{tReq(supplierCreateRequest{}), tReq(supplierUpdateRequest{})},
	},
	{
		component: "Task",
		//   api_collab.go:2599 handleGetTask:    c.JSON(200, task) task is *db.Task (GetTask)
		//   api_collab.go:2439 handleCreateTask: c.JSON(201, t)    t is db.Task
		//   api_collab.go:2720 handleUpdateTask: c.JSON(200, updated) updated is *db.Task (GetTask); 2722 falls back to t from prepareTaskUpdate
		//   api_collab.go:2581 handleUpdateTaskNotes: c.JSON(200, after) after is *db.Task
		//   api_collab.go:2356 handleListTasks:  {"data": tasks} tasks is []db.Task (PaginatedTasks); inbox_group and needs_action are filled only here
		shapes:   []contractShape{shape(tReq(db.Task{}), "api_collab.go:2599 handleGetTask: c.JSON(200, task), task is *db.Task")},
		requests: []reflect.Type{tReq(taskCreateRequest{}), tReq(taskUpdateRequest{})},
	},
	{
		component: "ChangeRequest",
		//   api_collab.go:2896 handleGetChange:    c.JSON(200, cr)      cr is *db.ChangeRequest (GetChangeRequest)
		//   api_collab.go:2968 handleCreateChange: c.JSON(201, cr)      cr is db.ChangeRequest (re-fetched)
		//   api_collab.go:3166 handleUpdateChange: c.JSON(200, updated) updated is *db.ChangeRequest
		//   api_collab.go:2867 handleListChanges:  {"data": changes} changes is []db.ChangeRequest (PaginatedChangeRequests); needs_action filled only here
		shapes:   []contractShape{shape(tReq(db.ChangeRequest{}), "api_collab.go:2896 handleGetChange: c.JSON(200, cr), cr is *db.ChangeRequest")},
		requests: []reflect.Type{tReq(changeCreateRequest{}), tReq(changeUpdateRequest{})},
	},
	{
		component: "AuditProgramme",
		//   api_audit.go:165 handleGetAuditProgramme:    c.JSON(200, p) p is *db.AuditProgramme (GetAuditProgramme)
		//   api_audit.go:152 handleCreateAuditProgramme: c.JSON(201, p) p is db.AuditProgramme
		//   api_audit.go:206 handleUpdateAuditProgramme: c.JSON(200, p) p is *db.AuditProgramme (GetAuditProgramme)
		//   api_audit.go:104 handleListAuditProgrammes:  {"data": programmes} programmes is []db.AuditProgramme (ListAuditProgrammes)
		shapes:   []contractShape{shape(tReq(db.AuditProgramme{}), "api_audit.go:165 handleGetAuditProgramme: c.JSON(200, p), p is *db.AuditProgramme")},
		requests: []reflect.Type{tReq(auditProgrammeCreateRequest{}), tReq(auditProgrammeUpdateRequest{})},
	},
	{
		component: "Audit",
		//   api_audit.go:347 handleGetAudit:    c.JSON(200, audit) audit is *db.Audit (GetAudit)
		//   api_audit.go:334 handleCreateAudit: c.JSON(201, out)   out is *db.Audit (GetAudit)
		//   api_audit.go:403 handleUpdateAudit: c.JSON(200, after) after is *db.Audit (GetAudit)
		//   api_audit.go:263 handleListAudits:  {"data": audits} audits is []db.Audit (ListAudits)
		shapes:   []contractShape{shape(tReq(db.Audit{}), "api_audit.go:347 handleGetAudit: c.JSON(200, audit), audit is *db.Audit")},
		requests: []reflect.Type{tReq(auditCreateRequest{}), tReq(auditUpdateRequest{})},
	},
	{
		component: "AuditItem",
		//   api_audit.go:559 handleCreateAuditItem: c.JSON(201, item) item is *db.AuditItem
		//   api_audit.go:607 handleUpdateAuditItem: c.JSON(200, out)  out is *db.AuditItem (GetAuditItem)
		//   api_audit.go:517 handleListAuditItems:  {"data": items} items is []db.AuditItem (ListAuditItems)
		shapes:   []contractShape{shape(tReq(db.AuditItem{}), "api_audit.go:607 handleUpdateAuditItem: c.JSON(200, out), out is *db.AuditItem")},
		requests: []reflect.Type{tReq(auditItemCreateRequest{}), tReq(auditItemUpdateRequest{})},
	},
	{
		component: "AuditFinding",
		//   api_audit.go:689 handleGetAuditFinding:    c.JSON(200, f)     f is *db.AuditFinding (GetAuditFinding)
		//   api_audit.go:769 handleAddAuditFinding:    c.JSON(201, out)   out is *db.AuditFinding (GetAuditFinding)
		//   api_audit.go:845 handleUpdateAuditFinding: c.JSON(200, after) after is *db.AuditFinding
		//   api_audit.go:627 handleListAuditFindingsForAudit: {"data": findings} findings is []db.AuditFinding (ListAuditFindings)
		//   api_audit.go:667 handlePaginatedAuditFindings: {"data": ..., ...} rows are []db.AuditFinding (PaginatedAuditFindings)
		shapes:   []contractShape{shape(tReq(db.AuditFinding{}), "api_audit.go:689 handleGetAuditFinding: c.JSON(200, f), f is *db.AuditFinding")},
		requests: []reflect.Type{tReq(auditFindingCreateRequest{}), tReq(auditFindingUpdateRequest{})},
	},
	{
		component: "LegalRequirement",
		//   api_legal.go:183 handleGetLegal:    c.JSON(200, lr)    lr is *db.LegalRequirement (GetLegalRequirement)
		//   api_legal.go:168 handleCreateLegal: c.JSON(201, lr)    lr is db.LegalRequirement
		//   api_legal.go:341 handleUpdateLegal: c.JSON(200, after) after is *db.LegalRequirement (GetLegalRequirementTx)
		//   api_legal.go:93  handleListLegal:   {"data": ...} rows are []db.LegalRequirement (PaginatedLegalRequirements)
		shapes:   []contractShape{shape(tReq(db.LegalRequirement{}), "api_legal.go:183 handleGetLegal: c.JSON(200, lr), lr is *db.LegalRequirement")},
		requests: []reflect.Type{tReq(legalCreateRequest{}), tReq(legalUpdateRequest{})},
	},
	{
		component: "Incident",
		//   api_incidents.go:222 handleGetIncident:    c.JSON(200, inc)    inc is *db.Incident (GetIncident)
		//   api_incidents.go:207 handleCreateIncident: c.JSON(201, inc)    inc is db.Incident (re-fetched)
		//   api_incidents.go:406 handleUpdateIncident: c.JSON(200, after)  after is *db.Incident (GetIncidentTx)
		//   api_incidents.go:660 handleUpdateIncidentProgress: c.JSON(200, after) after is *db.Incident
		//   api_incidents.go:99  handleListIncidents:  {"data": ...} rows are []db.Incident (PaginatedIncidents)
		shapes:   []contractShape{shape(tReq(db.Incident{}), "api_incidents.go:222 handleGetIncident: c.JSON(200, inc), inc is *db.Incident")},
		requests: []reflect.Type{tReq(incidentCreateRequest{}), tReq(incidentUpdateRequest{})},
	},
	{
		component: "CorrectiveAction",
		//   api_corrective.go:187 handleGetCorrectiveAction:    c.JSON(200, ca)    ca is *db.CorrectiveAction (GetCorrectiveAction)
		//   api_corrective.go:172 handleCreateCorrectiveAction: c.JSON(201, ca)    ca is db.CorrectiveAction (re-fetched)
		//   api_corrective.go:320 handleUpdateCorrectiveAction: c.JSON(200, after) after is *db.CorrectiveAction (GetCorrectiveActionTx)
		//   api_corrective.go:505 handleUpdateCorrectiveActionProgress: c.JSON(200, after) after is *db.CorrectiveAction
		//   api_corrective.go:75  handleListCorrectiveActions: {"data": ...} rows are []db.CorrectiveAction (PaginatedCorrectiveActions)
		shapes:   []contractShape{shape(tReq(db.CorrectiveAction{}), "api_corrective.go:187 handleGetCorrectiveAction: c.JSON(200, ca), ca is *db.CorrectiveAction")},
		requests: []reflect.Type{tReq(correctiveActionCreateRequest{}), tReq(correctiveActionUpdateRequest{})},
	},
	{
		component: "Program",
		//   api_objectives.go:197 handleGetProgram:    c.JSON(200, p)     p is *db.Program (GetProgram)
		//   api_objectives.go:169 handleCreateProgram: c.JSON(201, p)     p is db.Program
		//   api_objectives.go:256 handleUpdateProgram: c.JSON(200, after) after is *db.Program
		//   api_objectives.go:102 handleListPrograms:  {"data": programs} programs is []db.Program (ListPrograms)
		shapes:   []contractShape{shape(tReq(db.Program{}), "api_objectives.go:197 handleGetProgram: c.JSON(200, p), p is *db.Program")},
		requests: []reflect.Type{tReq(programCreateRequest{}), tReq(programUpdateRequest{})},
	},
	{
		component: "Objective",
		//   api_objectives.go:453 handleGetObjective:    c.JSON(200, o)     o is *db.Objective (GetObjective)
		//   api_objectives.go:438 handleCreateObjective: c.JSON(201, o)     o is db.Objective
		//   api_objectives.go:567 handleUpdateObjective: c.JSON(200, after) after is *db.Objective
		//   api_objectives.go:348 handleListObjectives:  {"data": ...} rows are []db.Objective (PaginatedObjectives)
		shapes:   []contractShape{shape(tReq(db.Objective{}), "api_objectives.go:453 handleGetObjective: c.JSON(200, o), o is *db.Objective")},
		requests: []reflect.Type{tReq(objectiveCreateRequest{}), tReq(objectiveUpdateRequest{})},
	},
	{
		component: "Checkin",
		//   api_objectives.go:802 handleCreateCheckin: c.JSON(201, ci)    ci is db.Checkin
		//   api_objectives.go:868 handleUpdateCheckin: c.JSON(200, after) after is *db.Checkin
		//   api_objectives.go:731 handleListCheckins:  {"data": checkins} checkins is []db.Checkin (ListCheckins)
		shapes:   []contractShape{shape(tReq(db.Checkin{}), "api_objectives.go:802 handleCreateCheckin: c.JSON(201, ci), ci is db.Checkin")},
		requests: []reflect.Type{tReq(checkinCreateRequest{}), tReq(checkinUpdateRequest{})},
	},
	{
		component: "Evidence",
		//   api_objectives.go:982 handleUploadEvidence: c.JSON(201, ev) ev is db.Evidence (multipart upload, no JSON request DTO)
		//   api_objectives.go:996 handleListEvidence:   {"data": evidence} evidence is []db.Evidence (ListEvidence)
		shapes: []contractShape{shape(tReq(db.Evidence{}), "api_objectives.go:982 handleUploadEvidence: c.JSON(201, ev), ev is db.Evidence")},
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
	"DocumentVersion": true, "Review": true, "Comment": true,
	"Approval": true, "Suggestion": true, "ImplementationStatus": true,
	"Notification": true, "EntityReference": true, "ChangelogEntry": true,
	"OIDCProvider": true,
}

// neverNullKeys are JSON keys the database always fills; see the header.
var neverNullKeys = map[string]bool{"created_at": true, "updated_at": true}

type specDoc struct {
	schemas specSchemas
	// requestBodies maps a component to the "METHOD /path" operations that
	// reference it directly as their request body.
	requestBodies map[string][]string
}

type specSchemas map[string]any

func loadSpec(t *testing.T) specDoc {
	t.Helper()
	var doc struct {
		Paths      map[string]map[string]any `yaml:"paths"`
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
	out := specDoc{schemas: doc.Components.Schemas, requestBodies: map[string][]string{}}
	const prefix = "#/components/schemas/"
	for path, item := range doc.Paths {
		for method, op := range item {
			opm, ok := op.(map[string]any)
			if !ok {
				continue
			}
			rb, _ := opm["requestBody"].(map[string]any)
			content, _ := rb["content"].(map[string]any)
			for _, mt := range content {
				mtm, _ := mt.(map[string]any)
				sch, _ := mtm["schema"].(map[string]any)
				if ref, ok := sch["$ref"].(string); ok && strings.HasPrefix(ref, prefix) {
					name := strings.TrimPrefix(ref, prefix)
					out.requestBodies[name] = append(out.requestBodies[name], strings.ToUpper(method)+" "+path)
				}
			}
		}
	}
	for _, ops := range out.requestBodies {
		sort.Strings(ops)
	}
	return out
}

func loadSpecSchemas(t *testing.T) specSchemas { return loadSpec(t).schemas }

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

func flag(m map[string]any, name string) bool {
	b, _ := m[name].(bool)
	return b
}

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

// objSeen accumulates, across shapes, which keys the responses emitted for one
// object path.
type objSeen struct {
	sch    map[string]any
	shapes int
	keys   map[string]int // key -> number of shapes that emitted it
}

// checkFull compares one fully populated response value with its schema:
// types, and keys the spec does not document. Keys the response emitted are
// recorded in seen; "documented but absent" is judged after all shapes ran.
func (s specSchemas) checkFull(path string, val any, node any, seen map[string]*objSeen, errs *[]string) {
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
					s.checkFull(path+"."+k, v[k], ap, seen, errs)
				}
			}
			return
		}
		o := seen[path]
		if o == nil {
			o = &objSeen{sch: sch, keys: map[string]int{}}
			seen[path] = o
		}
		o.shapes++
		for _, k := range sortedKeys(v) {
			o.keys[k]++
			p, ok := props[k]
			if !ok {
				*errs = append(*errs, fmt.Sprintf("%s: response has %q but the spec does not document it", path, k))
				continue
			}
			if ps, err := s.resolve(p); err == nil && flag(ps, "writeOnly") {
				*errs = append(*errs, fmt.Sprintf("%s.%s: marked writeOnly but the response emits it", path, k))
			}
			s.checkFull(path+"."+k, v[k], p, seen, errs)
		}
	case []any:
		if items, ok := sch["items"]; ok {
			for i, e := range v {
				s.checkFull(fmt.Sprintf("%s[%d]", path, i), e, items, seen, errs)
			}
		}
	}
}

// checkUnion reports documented keys no shape emitted (writeOnly keys are
// request-only and exempt) and required keys some shape did not emit.
func (s specSchemas) checkUnion(seen map[string]*objSeen, errs *[]string) {
	for _, path := range sortedKeys(seen) {
		o := seen[path]
		props, _ := o.sch["properties"].(map[string]any)
		for _, k := range sortedKeys(props) {
			ps, err := s.resolve(props[k])
			if err != nil {
				*errs = append(*errs, fmt.Sprintf("%s.%s: %v", path, k, err))
				continue
			}
			if o.keys[k] == 0 && !flag(ps, "writeOnly") {
				*errs = append(*errs, fmt.Sprintf("%s: spec documents %q but the response has no such key", path, k))
			}
			if o.keys[k] > 0 && o.keys[k] < o.shapes && isRequired(o.sch, k) {
				*errs = append(*errs, fmt.Sprintf("%s: %q is emitted by only some response shapes, so it must not be in required", path, k))
			}
		}
	}
}

// zeroFact records what the zero-value marshaling did to one key, per shape.
type zeroFact struct {
	sch                 map[string]any // the object schema holding the key
	prop                map[string]any
	null, omitted, vals int
}

// collectZero compares the zero-value marshaling (full is the populated one,
// used to learn which keys exist) with the schema's nullability and required.
// Facts are judged after all shapes ran (see judgeZero).
func (s specSchemas) collectZero(path string, zero, full map[string]any, node any, facts map[string]*zeroFact) {
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
		f := facts[p]
		if f == nil {
			f = &zeroFact{sch: sch, prop: ps}
			facts[p] = f
		}
		zv, present := zero[k]
		switch {
		case !present:
			f.omitted++
		case zv == nil:
			f.null++
		default:
			f.vals++
			zm, zok := zv.(map[string]any)
			fm, fok := full[k].(map[string]any)
			if zok && fok {
				s.collectZero(p, zm, fm, ps, facts)
			}
		}
	}
}

func (s specSchemas) judgeZero(facts map[string]*zeroFact, errs *[]string) {
	for _, p := range sortedKeys(facts) {
		f := facts[p]
		key := p[strings.LastIndex(p, ".")+1:]
		typ := strings.Join(schemaTypes(f.prop), "|")
		switch {
		case neverNullKeys[key]:
			if allowsNull(f.prop) {
				*errs = append(*errs, fmt.Sprintf("%s: the database always sets this key, spec type must not include null", p))
			}
		case f.null > 0:
			if !allowsNull(f.prop) {
				*errs = append(*errs, fmt.Sprintf("%s: emitted null at its zero value, spec type %s does not include null", p, typ))
			}
		default:
			if allowsNull(f.prop) {
				why := "the response never emits null for it"
				if f.omitted > 0 && f.vals == 0 {
					why = "it is omitted when empty (omitempty), so the type must not include null"
				}
				*errs = append(*errs, fmt.Sprintf("%s: spec type includes null but %s", p, why))
			}
		}
		if f.omitted > 0 && isRequired(f.sch, key) {
			*errs = append(*errs, fmt.Sprintf("%s: omitted when empty (omitempty), so it must not be in required", p))
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

// jsonFields returns the JSON keys a struct accepts or emits at the top level
// with their Go types.
func jsonFields(t reflect.Type, into map[string]reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			jsonFields(f.Type, into)
			continue
		}
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		into[name] = f.Type
	}
}

// acceptedKinds returns the JSON types a request field of Go type t decodes.
// nil means "anything" (interfaces, raw JSON).
func acceptedKinds(t reflect.Type) []string {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && strings.HasPrefix(t.Name(), "Optional[") {
		if f, ok := t.FieldByName("Value"); ok {
			return acceptedKinds(f.Type)
		}
	}
	switch t {
	case epochType:
		return []string{"integer", "string"} // Epoch.UnmarshalJSON: seconds or a date string
	case timeType:
		return []string{"string"}
	case rawJSONType:
		return nil
	}
	switch t.Kind() {
	case reflect.String:
		return []string{"string"}
	case reflect.Bool:
		return []string{"boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []string{"integer"}
	case reflect.Float32, reflect.Float64:
		return []string{"number", "integer"}
	case reflect.Slice, reflect.Array:
		return []string{"array"}
	case reflect.Map, reflect.Struct:
		return []string{"object"}
	}
	return nil
}

func TestOpenAPIContract(t *testing.T) {
	spec := loadSpec(t)
	schemas := spec.schemas
	for _, e := range contractTable {
		t.Run(e.component, func(t *testing.T) {
			node, ok := schemas[e.component]
			if !ok {
				t.Fatalf("%s: in contractTable but not in components/schemas", e.component)
			}
			if len(e.shapes) == 0 {
				t.Fatalf("%s: contractTable entry has no shapes", e.component)
			}
			var errs []string

			// Request-body coverage: a component some operation takes as its
			// request body must say which DTOs it binds, or the readOnly and
			// writeOnly checks below would silently be skipped.
			if ops := spec.requestBodies[e.component]; len(ops) > 0 && len(e.requests) == 0 {
				errs = append(errs, fmt.Sprintf("component %s is a request body for %s but lists no request DTOs", e.component, ops[0]))
			}

			seen := map[string]*objSeen{}
			facts := map[string]*zeroFact{}
			emitted := map[string]bool{}
			for _, sh := range e.shapes {
				if sh.handler == "" {
					errs = append(errs, fmt.Sprintf("%s: a shape has no handler file:line comment", e.component))
				}
				filled := reflect.New(sh.typ)
				fill(filled.Elem(), 0)
				full := marshalToMap(t, filled.Interface())
				zero := marshalToMap(t, reflect.New(sh.typ).Interface())
				for k := range full {
					emitted[k] = true
				}
				schemas.checkFull(e.component, full, node, seen, &errs)
				schemas.collectZero(e.component, zero, full, node, facts)
			}
			schemas.checkUnion(seen, &errs)
			schemas.judgeZero(facts, &errs)

			sch, err := schemas.resolve(node)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", e.component, err))
			}
			props, _ := sch["properties"].(map[string]any)

			if len(e.requests) > 0 && err == nil {
				accepted := map[string]reflect.Type{}
				for _, r := range e.requests {
					jsonFields(r, accepted)
				}
				serverSet := map[string]bool{}
				for _, k := range e.serverSet {
					serverSet[k] = true
				}
				for _, k := range sortedKeys(accepted) {
					if serverSet[k] {
						continue
					}
					if _, ok := props[k]; !ok {
						errs = append(errs, fmt.Sprintf("%s: a request body accepts %q but the spec does not document it", e.component, k))
					}
				}
				for _, k := range sortedKeys(props) {
					ps, err := schemas.resolve(props[k])
					if err != nil {
						continue
					}
					ro, wo := flag(ps, "readOnly"), flag(ps, "writeOnly")
					_, acc := accepted[k]
					acc = acc && !serverSet[k]
					switch {
					case ro && acc:
						errs = append(errs, fmt.Sprintf("%s.%s: marked readOnly but the request body accepts it", e.component, k))
					case !ro && !acc && emitted[k]:
						errs = append(errs, fmt.Sprintf("%s.%s: not readOnly but the request body does not accept it", e.component, k))
					case wo && emitted[k]:
						// reported by checkFull
					case wo && !acc:
						errs = append(errs, fmt.Sprintf("%s.%s: marked writeOnly but no request body accepts it", e.component, k))
					case !wo && acc && !emitted[k]:
						errs = append(errs, fmt.Sprintf("%s.%s: accepted by a request body but never emitted, so it must be writeOnly", e.component, k))
					}
					// Request field type: the schema type is what a client
					// sends, so the DTO field must decode it.
					if rt, ok := accepted[k]; ok && acc && !ro {
						if kinds := acceptedKinds(rt); kinds != nil {
							for _, st := range schemaTypes(ps) {
								if st != "null" && !hasType(kinds, st) {
									errs = append(errs, fmt.Sprintf("%s.%s: spec type %s but the request field (%s) only decodes %s", e.component, k, st, rt, strings.Join(kinds, "|")))
								}
							}
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
		if inTable[e.component] {
			t.Errorf("component %s appears twice in contractTable", e.component)
		}
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
