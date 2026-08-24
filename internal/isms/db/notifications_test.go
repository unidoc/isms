package db

import (
	"encoding/json"
	"testing"
)

func TestNotificationContentParamsJSON(t *testing.T) {
	t.Run("no params marshals to SQL NULL, not an empty object", func(t *testing.T) {
		// A JSONB '{}' and a NULL read back differently on the client: '{}' would
		// make an unkeyed legacy row look like it carried params.
		if got := (NotificationContent{}).paramsJSON(); got != nil {
			t.Fatalf("empty params: want nil, got %q", got)
		}
		if got := (NotificationContent{Params: map[string]any{}}).paramsJSON(); got != nil {
			t.Fatalf("zero-length map: want nil, got %q", got)
		}
	})

	t.Run("params round-trip through JSON", func(t *testing.T) {
		c := NotificationContent{Params: map[string]any{
			"actor":    "alice@example.com",
			"title":    "Access Control Policy",
			"severity": "critical",
			"round":    3,
			// User-authored text, which must survive verbatim — including the
			// characters that would break naive string building.
			"note": "please check \"section 4\" & the annex\nthanks",
		}}
		raw := c.paramsJSON()
		if raw == nil {
			t.Fatal("want JSON, got nil")
		}
		var back map[string]any
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if back["note"] != c.Params["note"] {
			t.Errorf("note not preserved verbatim:\n got %q\nwant %q", back["note"], c.Params["note"])
		}
		if back["severity"] != "critical" {
			t.Errorf("severity = %v, want critical", back["severity"])
		}
		// Numbers come back as float64 through encoding/json; the renderer
		// interpolates them as-is, so this is expected rather than a defect.
		if back["round"] != float64(3) {
			t.Errorf("round = %#v, want float64(3)", back["round"])
		}
	})

	t.Run("unmarshalable params are dropped, not fatal", func(t *testing.T) {
		// The English Title still renders, so losing a translation beats losing
		// the notification.
		c := NotificationContent{Params: map[string]any{"bad": func() {}}}
		if got := c.paramsJSON(); got != nil {
			t.Fatalf("want nil on marshal failure, got %q", got)
		}
	})
}

func TestNotificationJSONOmitsKeysWhenAbsent(t *testing.T) {
	// Legacy rows and agent rows must serialise exactly as they did before the
	// key columns existed, or existing clients see fields they do not expect.
	raw, err := json.Marshal(Notification{ID: 1, Title: "AI review escalated"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, absent := range []string{"title_key", "body_key", "params"} {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := m[absent]; ok {
			t.Errorf("unkeyed notification should omit %q, got %s", absent, raw)
		}
	}

	keyed, err := json.Marshal(Notification{
		ID:       2,
		Title:    "New critical incident: Outage",
		TitleKey: "notifications.incident_new",
		Params:   map[string]any{"severity": "critical", "title": "Outage"},
	})
	if err != nil {
		t.Fatalf("marshal keyed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(keyed, &m); err != nil {
		t.Fatalf("unmarshal keyed: %v", err)
	}
	if m["title_key"] != "notifications.incident_new" {
		t.Errorf("title_key = %v", m["title_key"])
	}
	// The English original stays alongside the key — it is the fallback.
	if m["title"] != "New critical incident: Outage" {
		t.Errorf("title = %v, want the English original preserved", m["title"])
	}
}
