package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// CustomFieldDef is one admin-defined extra field on a risk. Key is a slug
// frozen at creation and Type is immutable after creation (risks store values
// under the key; retyping would require migrating stored values, which is
// deliberately not supported — delete and recreate instead). Label and Options
// are org-authored display text and are NEVER routed through i18n.
type CustomFieldDef struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // text | number | date | select
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"` // select only
}

// NOTE: Required is enforced on handleAddRisk and handleUpdateRisk, but
// deliberately NOT on the suggestion-apply risk-creation path — agents can't
// fill out a form. See "Required fields are in scope" in the implementation
// plan for #213/#216.

// CustomFieldTypes lists the field types supported in v1.
var CustomFieldTypes = []string{"text", "number", "date", "select"}

// Bounds for a stored risk_custom_fields payload.
const (
	maxCustomFields       = 30
	maxCustomFieldKey     = 64
	maxCustomFieldLabel   = 100
	maxCustomFieldOptions = 50
	maxCustomFieldOption  = 100
	maxCustomFieldText    = 500
)

// ParseCustomFieldDefs parses and validates a risk_custom_fields setting
// payload. Shared by the settings PUT (which rejects bad input with a 400)
// and by CustomFieldDefsFor (which falls back to an empty slice instead).
func ParseCustomFieldDefs(raw string) ([]CustomFieldDef, error) {
	var defs []CustomFieldDef
	if err := json.Unmarshal([]byte(raw), &defs); err != nil {
		return nil, fmt.Errorf("risk_custom_fields must be a JSON array of {key, label, type, required, options}: %w", err)
	}
	if len(defs) > maxCustomFields {
		return nil, fmt.Errorf("risk_custom_fields must contain at most %d fields, got %d", maxCustomFields, len(defs))
	}

	seen := make(map[string]struct{}, len(defs))
	out := make([]CustomFieldDef, 0, len(defs))
	for i, d := range defs {
		if utf8.RuneCountInString(d.Key) > maxCustomFieldKey {
			return nil, fmt.Errorf("field %d: key must be at most %d characters", i+1, maxCustomFieldKey)
		}
		if !riskCategoryKeyPattern.MatchString(d.Key) {
			return nil, fmt.Errorf("field %d: key %q must be lowercase words separated by underscores", i+1, d.Key)
		}
		lower := strings.ToLower(d.Key)
		if _, dup := seen[lower]; dup {
			return nil, fmt.Errorf("field %d: duplicate key %q", i+1, d.Key)
		}
		seen[lower] = struct{}{}

		label := strings.TrimSpace(d.Label)
		if label == "" {
			return nil, fmt.Errorf("field %d (%s): label is required", i+1, d.Key)
		}
		if utf8.RuneCountInString(label) > maxCustomFieldLabel {
			return nil, fmt.Errorf("field %d (%s): label must be at most %d characters", i+1, d.Key, maxCustomFieldLabel)
		}

		typeOK := false
		for _, t := range CustomFieldTypes {
			if d.Type == t {
				typeOK = true
				break
			}
		}
		if !typeOK {
			return nil, fmt.Errorf("field %d (%s): type %q is not one of %v", i+1, d.Key, d.Type, CustomFieldTypes)
		}

		var options []string
		if d.Type == "select" {
			if len(d.Options) == 0 {
				return nil, fmt.Errorf("field %d (%s): select fields must have at least one option", i+1, d.Key)
			}
			if len(d.Options) > maxCustomFieldOptions {
				return nil, fmt.Errorf("field %d (%s): must have at most %d options, got %d", i+1, d.Key, maxCustomFieldOptions, len(d.Options))
			}
			optSeen := make(map[string]struct{}, len(d.Options))
			options = make([]string, 0, len(d.Options))
			for j, o := range d.Options {
				opt := strings.TrimSpace(o)
				if opt == "" {
					return nil, fmt.Errorf("field %d (%s): option %d is empty", i+1, d.Key, j+1)
				}
				if utf8.RuneCountInString(opt) > maxCustomFieldOption {
					return nil, fmt.Errorf("field %d (%s): option %d must be at most %d characters", i+1, d.Key, j+1, maxCustomFieldOption)
				}
				optLower := strings.ToLower(opt)
				if _, dup := optSeen[optLower]; dup {
					return nil, fmt.Errorf("field %d (%s): duplicate option %q", i+1, d.Key, opt)
				}
				optSeen[optLower] = struct{}{}
				options = append(options, opt)
			}
		} else if len(d.Options) > 0 {
			return nil, fmt.Errorf("field %d (%s): options are only allowed for select fields", i+1, d.Key)
		}

		out = append(out, CustomFieldDef{
			Key:      d.Key,
			Label:    label,
			Type:     d.Type,
			Required: d.Required,
			Options:  options,
		})
	}
	return out, nil
}

// CustomFieldDefsFor returns the custom field definitions configured for an
// org. Unlike RiskCategoriesFor there is no built-in fallback list — the
// normal state is "no custom fields defined" — so any failure (unseeded
// setting, empty value, malformed JSON) degrades to an empty slice, never an
// error, so a bad stored value can never block risk creation.
func (d *DB) CustomFieldDefsFor(ctx context.Context, orgID int) ([]CustomFieldDef, error) {
	raw, err := d.GetOrgSetting(ctx, orgID, "risk_custom_fields")
	if err != nil {
		log.Printf("custom fields: reading setting for org %d: %v (using none)", orgID, err)
		return []CustomFieldDef{}, nil
	}
	if strings.TrimSpace(raw) == "" {
		return []CustomFieldDef{}, nil
	}
	defs, err := ParseCustomFieldDefs(raw)
	if err != nil {
		log.Printf("custom fields: stored value for org %d is invalid: %v (using none)", orgID, err)
		return []CustomFieldDef{}, nil
	}
	if defs == nil {
		defs = []CustomFieldDef{}
	}
	return defs, nil
}

func customFieldDefByKey(defs []CustomFieldDef, key string) (CustomFieldDef, bool) {
	for _, d := range defs {
		if d.Key == key {
			return d, true
		}
	}
	return CustomFieldDef{}, false
}

// ValidateCustomFieldValues is the single validation gate for a risk's custom
// field values. checkRequired lets callers state their intent explicitly:
// handleAddRisk and handleUpdateRisk pass true; the suggestion-apply
// risk-creation path passes false because agents cannot fill out a form (see
// "Required fields are in scope" in the implementation plan for #213/#216).
func ValidateCustomFieldValues(defs []CustomFieldDef, values map[string]any, checkRequired bool) error {
	for k, v := range values {
		def, ok := customFieldDefByKey(defs, k)
		if !ok {
			return fmt.Errorf("unknown custom field %q", k)
		}
		if v == nil {
			continue
		}
		switch def.Type {
		case "text":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("custom field %q must be a string", k)
			}
			if utf8.RuneCountInString(strings.TrimSpace(s)) > maxCustomFieldText {
				return fmt.Errorf("custom field %q must be at most %d characters", k, maxCustomFieldText)
			}
		case "number":
			if s, ok := v.(string); ok {
				if s == "" {
					continue
				}
				return fmt.Errorf("custom field %q must be a number, not a string", k)
			}
			if _, ok := v.(float64); !ok {
				return fmt.Errorf("custom field %q must be a number", k)
			}
		case "date":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("custom field %q must be a date string", k)
			}
			if s == "" {
				continue
			}
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return fmt.Errorf("custom field %q must be a date in YYYY-MM-DD format", k)
			}
		case "select":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("custom field %q must be a string", k)
			}
			if s == "" {
				continue
			}
			found := false
			for _, opt := range def.Options {
				if opt == s {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("custom field %q value %q is not one of the configured options", k, s)
			}
		}
	}

	if checkRequired {
		if err := RequiredCustomFieldsSatisfied(defs, values); err != nil {
			return err
		}
	}
	return nil
}

// RequiredCustomFieldsSatisfied checks only that every required def has a
// non-empty value in values. Unlike ValidateCustomFieldValues, it does NOT
// reject keys in values that have no matching def — callers use it to enforce
// "required" over a full merged map that may still carry orphaned values from
// a deleted definition, without re-triggering the unknown-key check on those
// orphans (that check is already covered, changed-values-only, by a prior
// ValidateCustomFieldValues call). See handleUpdateRisk and "The one rule that
// is easy to get wrong" in the implementation plan for #213/#216.
func RequiredCustomFieldsSatisfied(defs []CustomFieldDef, values map[string]any) error {
	for _, def := range defs {
		if !def.Required {
			continue
		}
		v, ok := values[def.Key]
		if !ok || v == nil {
			return fmt.Errorf("custom field %q is required", def.Key)
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf("custom field %q is required", def.Key)
		}
	}
	return nil
}

// NormalizeCustomFieldValues drops keys whose value is nil or "" so cleared
// fields do not linger in the JSONB, and trims string values. It drops only
// nil/empty values — it must NEVER remove a key just because no definition
// matches it. Such a key is an orphan left behind by a deleted definition and
// must survive, or every edit would silently cascade-delete the orphans the
// orphan-not-cascade rule exists to preserve. defs is used only to decide
// trimming/coercion, never as a whitelist.
func NormalizeCustomFieldValues(defs []CustomFieldDef, values map[string]any) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out[k] = s
			continue
		}
		out[k] = v
	}
	return out
}

// customFieldsArg makes a values map safe to pass to pgx: a nil map would
// write SQL NULL, and the column is NOT NULL.
func customFieldsArg(v map[string]any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	return v
}

// sortedCustomFieldKeys is a small helper used by the changelog code to make
// entry order deterministic.
func sortedCustomFieldKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
