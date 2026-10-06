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

// updateRequestTypes maps an entity type to a constructor for the HTTP update
// request type its update apply handler decodes payload.fields into (#200).
// TestUpdateHandlersDecodeHTTPRequestTypes keeps it in step with the update
// apply handlers in the registry.
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

// createPayloadTypes maps an entity type to the payload type its create apply
// handler decodes into: the HTTP create request type, embedded, plus any
// compatibility aliases. A create payload is strictly decoded into it at
// create/edit time exactly as at apply time, so an unknown key or a wrongly
// typed value is refused. TestCreatePayloadsEmbedHTTPRequestTypes checks every
// entry and that every create handler has one.
var createPayloadTypes = map[string]reflect.Type{
	"risk":              reflect.TypeOf(riskCreatePayload{}),
	"incident":          reflect.TypeOf(incidentCreatePayload{}),
	"supplier":          reflect.TypeOf(supplierCreatePayload{}),
	"legal_requirement": reflect.TypeOf(legalCreatePayload{}),
	"change_request":    reflect.TypeOf(changeCreatePayload{}),
	"corrective_action": reflect.TypeOf(correctiveActionCreatePayload{}),
	"task":              reflect.TypeOf(taskCreatePayload{}),
	"objective":         reflect.TypeOf(objectiveCreatePayload{}),
	"program":           reflect.TypeOf(programCreatePayload{}),
	"system":            reflect.TypeOf(systemCreatePayload{}),
	"asset":             reflect.TypeOf(assetCreatePayload{}),
	"audit_finding":     reflect.TypeOf(auditFindingCreatePayload{}),
}

// strictCreatePayload decodes raw into a fresh value of the create payload type
// registered for entityType; keys the type does not declare are refused.
func strictCreatePayload(entityType string, raw json.RawMessage) error {
	t, ok := createPayloadTypes[entityType]
	if !ok {
		return nil
	}
	return decodeSuggestionPayload(raw, reflect.New(t).Interface())
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
		return strictCreatePayload(entityType, raw)
	}
	return nil
}

const updateShapeExample = `{"fields":{"status":"resolved"}}`

// validateUpdatePayload checks an update payload against the fields its apply
// handler writes. With requireFields, a payload proposing no values is refused
// ("nothing to apply"); without it (create/edit time) that is a free-text
// suggestion and is allowed.
func validateUpdatePayload(entityType string, raw json.RawMessage, requireFields bool) error {
	ctor, ok := updateRequestTypes[entityType]
	if !ok {
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

	return decodeUpdateFields(entityType, raw, ctor())
}

func updateShapeError(stray []string) error {
	return echo.NewHTTPError(http.StatusBadRequest,
		`update payload values must be nested under "fields", e.g. `+updateShapeExample+
			"; unexpected top-level key(s): "+strings.Join(stray, ", "))
}
