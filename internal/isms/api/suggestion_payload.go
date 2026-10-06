package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"
)

// Suggestion payload checks (#298, #200). A suggestion may only be marked
// "applied" when its payload proposes values the apply handler actually writes.
// Anything a handler would silently drop is refused with a 400 instead: at
// create/edit time so a bad payload never reaches a manager, and again at apply
// time for suggestions stored before these checks existed. A 400 inside the
// apply transaction rolls back and leaves the suggestion open.

// payloadKind is the JSON type an update-suggestion field must carry.
type payloadKind int

const (
	kindString payloadKind = iota
	kindBool
)

func (k payloadKind) String() string {
	if k == kindBool {
		return "true or false"
	}
	return "a string"
}

// updatePayloadFields lists, per entity type, exactly the keys its update apply
// handler reads from payload.fields, and the JSON type each must be. Keep it in
// step with the handlers in api_suggestions.go: TestUpdatePayloadFieldsMatchHandlers
// fails if a handler reads a key missing here, or a key listed here is never read.
var updatePayloadFields = map[string]map[string]payloadKind{}

// updateRequestTypes maps an entity type to a constructor for the HTTP update
// request type its update apply handler decodes payload.fields into (#200).
// Entities move here from updatePayloadFields one by one (Steps 3-8); when
// updatePayloadFields is empty it is deleted (Step 8.3).
var updateRequestTypes = map[string]func() any{
	"asset":             func() any { return &assetUpdateRequest{} },
	"system":            func() any { return &systemUpdateRequest{} },
	"supplier":          func() any { return &supplierUpdateRequest{} },
	"risk":              func() any { return &riskUpdateRequest{} },
	"legal_requirement": func() any { return &legalUpdateRequest{} },
	"objective":         func() any { return &objectiveUpdateRequest{} },
	"incident":          func() any { return &incidentUpdateRequest{} },
	"corrective_action": func() any { return &correctiveActionUpdateRequest{} },
	"change_request":    func() any { return &changeUpdateRequest{} },
	"task":              func() any { return &taskUpdateRequest{} },
	"audit_finding":     func() any { return &auditFindingUpdateRequest{} },
}

// jsonFieldTypes maps each json tag name of struct type t to its field type.
func jsonFieldTypes(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = f.Type
	}
	return out
}

func isOptionalType(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && strings.HasPrefix(t.Name(), "Optional[")
}

// supportedUpdateFields lists the keys an update suggestion of entityType may carry.
func supportedUpdateFields(entityType string) []string {
	var out []string
	if ctor, ok := updateRequestTypes[entityType]; ok {
		for k := range jsonFieldTypes(reflect.TypeOf(ctor()).Elem()) {
			out = append(out, k)
		}
	} else {
		for k := range updatePayloadFields[entityType] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// decodeUpdateFields strictly decodes the "fields" object of an update payload
// into dst, which must be a pointer to an HTTP update request struct. It refuses:
//   - keys dst has no json tag for, naming them and the supported list;
//   - null for any key whose field is not Optional[T] (null means "clear" only
//     where PUT supports clearing; elsewhere it would be dropped silently);
//   - values of the wrong JSON type.
//
// Shape checks (stray top-level keys, empty fields) stay in validateUpdatePayload.
func decodeUpdateFields(entityType string, raw json.RawMessage, dst any) error {
	var top struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if !isJSONNull(raw) {
		if err := json.Unmarshal(raw, &top); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "update payload must be a JSON object, e.g. "+updateShapeExample)
		}
	}
	tags := jsonFieldTypes(reflect.TypeOf(dst).Elem())
	keys := make([]string, 0, len(top.Fields))
	for k := range top.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var unknown, nulls []string
	for _, k := range keys {
		ft, ok := tags[k]
		if !ok {
			unknown = append(unknown, k)
			continue
		}
		if isJSONNull(top.Fields[k]) && !isOptionalType(ft) {
			nulls = append(nulls, k)
		}
	}
	if len(unknown) > 0 {
		supported := make([]string, 0, len(tags))
		for k := range tags {
			supported = append(supported, k)
		}
		sort.Strings(supported)
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf(
			"%s update suggestions cannot apply field(s) %s (supported: %s)",
			entityType, strings.Join(unknown, ", "), strings.Join(supported, ", ")))
	}
	if len(nulls) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("field(s) %s cannot be null", strings.Join(nulls, ", ")))
	}
	if len(top.Fields) == 0 {
		return nil
	}
	b, err := json.Marshal(top.Fields)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid field value(s): "+err.Error())
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid field value(s): "+err.Error())
	}
	return nil
}

// createPayloadValidators strictly decodes a create payload into the same type
// its apply handler decodes into, so an unknown key or a wrongly typed value is
// refused at create time exactly as it would be at apply time.
// TestCreatePayloadValidatorsCoverRegistry fails if a create handler has no entry.
var createPayloadValidators = map[string]func(json.RawMessage) error{
	"risk":              strictCreatePayload[riskCreatePayload],
	"incident":          strictCreatePayload[incidentCreatePayload],
	"supplier":          strictCreatePayload[supplierCreatePayload],
	"legal_requirement": strictCreatePayload[legalCreatePayload],
	"change_request":    strictCreatePayload[changeCreatePayload],
	"corrective_action": strictCreatePayload[correctiveActionCreatePayload],
	"task":              strictCreatePayload[taskCreatePayload],
	"objective":         strictCreatePayload[objectiveCreatePayload],
	"program":           strictCreatePayload[programCreatePayload],
	"system":            strictCreatePayload[systemCreatePayload],
	"asset":             strictCreatePayload[assetCreatePayload],
	"audit_finding":     strictCreatePayload[auditFindingCreatePayload],
}

func strictCreatePayload[T any](raw json.RawMessage) error {
	var p T
	return decodeSuggestionPayload(raw, &p)
}

// isJSONNull reports whether raw is empty or the JSON literal null.
func isJSONNull(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || bytes.Equal(t, []byte("null"))
}

// decodeSuggestionPayload decodes a create payload into dst, refusing keys dst
// does not declare and values of the wrong type. An empty or null payload
// decodes as {}.
func decodeSuggestionPayload(raw json.RawMessage, dst any) error {
	data := []byte(bytes.TrimSpace(raw))
	if isJSONNull(raw) {
		data = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid create payload: "+err.Error())
	}
	return nil
}

// validateSuggestionPayload is the create/edit-time check. Free-text update
// suggestions (no proposed values at all) are allowed here; applying one is
// refused later by validateUpdatePayload with requireFields=true.
func validateSuggestionPayload(entityType, suggestionType string, raw json.RawMessage) error {
	switch suggestionType {
	case "update":
		return validateUpdatePayload(entityType, raw, false)
	case "create":
		if v := createPayloadValidators[entityType]; v != nil {
			return v(raw)
		}
	}
	return nil
}

const updateShapeExample = `{"fields":{"status":"resolved"}}`

// validateUpdatePayload checks an update payload against the fields its apply
// handler writes. With requireFields, a payload proposing no values is refused
// ("nothing to apply"); without it (create/edit time) that is a free-text
// suggestion and is allowed.
func validateUpdatePayload(entityType string, raw json.RawMessage, requireFields bool) error {
	allowed, legacy := updatePayloadFields[entityType]
	ctor, typed := updateRequestTypes[entityType]
	if !legacy && !typed {
		return nil // no update handler for this type; the registry check reports that
	}

	top := map[string]json.RawMessage{}
	if !isJSONNull(raw) {
		if err := json.Unmarshal(raw, &top); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "update payload must be a JSON object, e.g. "+updateShapeExample)
		}
	}
	var stray []string
	for k := range top {
		if k != "fields" {
			stray = append(stray, k)
		}
	}
	sort.Strings(stray)

	fields := map[string]json.RawMessage{}
	if rawFields, ok := top["fields"]; ok && !isJSONNull(rawFields) {
		if err := json.Unmarshal(rawFields, &fields); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, `update payload "fields" must be a JSON object, e.g. `+updateShapeExample)
		}
	}

	if len(fields) == 0 {
		if requireFields {
			msg := `nothing to apply: an update suggestion's proposed values must be under "fields", e.g. ` +
				updateShapeExample + `, and this one has none`
			if len(stray) > 0 {
				msg += "; top-level key(s) " + strings.Join(stray, ", ") + " are not applied"
			}
			msg += ". Act on it by hand and reject it with a reason, or edit it to add values."
			return echo.NewHTTPError(http.StatusBadRequest, msg)
		}
		if len(stray) > 0 {
			return updateShapeError(stray)
		}
		return nil // free-text suggestion
	}
	if len(stray) > 0 {
		return updateShapeError(stray)
	}

	if typed {
		return decodeUpdateFields(entityType, raw, ctor())
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var unknown, badType []string
	for _, k := range keys {
		kind, ok := allowed[k]
		if !ok {
			unknown = append(unknown, k)
			continue
		}
		if !jsonValueIsKind(fields[k], kind) {
			badType = append(badType, fmt.Sprintf("%s must be %s", k, kind))
		}
	}
	if len(unknown) > 0 {
		supported := make([]string, 0, len(allowed))
		for k := range allowed {
			supported = append(supported, k)
		}
		sort.Strings(supported)
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf(
			"%s update suggestions cannot apply field(s) %s (supported: %s)",
			entityType, strings.Join(unknown, ", "), strings.Join(supported, ", ")))
	}
	if len(badType) > 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid field value(s): "+strings.Join(badType, "; "))
	}
	return nil
}

func updateShapeError(stray []string) error {
	return echo.NewHTTPError(http.StatusBadRequest,
		`update payload values must be nested under "fields", e.g. `+updateShapeExample+
			"; unexpected top-level key(s): "+strings.Join(stray, ", "))
}

func jsonValueIsKind(raw json.RawMessage, kind payloadKind) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch kind {
	case kindBool:
		_, ok := v.(bool)
		return ok
	default:
		_, ok := v.(string)
		return ok
	}
}
