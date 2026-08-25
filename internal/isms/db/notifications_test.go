package db

import (
	"encoding/json"
	"testing"
)

func TestNotificationContentParamsJSON(t *testing.T) {
	t.Run("no params marshals to SQL NULL, not an empty object", func(t *testing.T) {
		// A JSONB '{}' and a NULL read back differently on the client: '{}' would
		// make an unkeyed legacy row look like it carried params.
		if got, ok := (NotificationContent{}).paramsJSON(); got != nil || !ok {
			t.Fatalf("empty params: want nil/ok, got %q/%v", got, ok)
		}
		if got, ok := (NotificationContent{Params: map[string]any{}}).paramsJSON(); got != nil || !ok {
			t.Fatalf("zero-length map: want nil/ok, got %q/%v", got, ok)
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
		raw, ok := c.paramsJSON()
		if raw == nil || !ok {
			t.Fatalf("want JSON/ok, got %q/%v", raw, ok)
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
		got, ok := c.paramsJSON()
		if got != nil {
			t.Fatalf("want nil on marshal failure, got %q", got)
		}
		if ok {
			t.Fatal("want ok=false on marshal failure, so the caller can drop the keys")
		}
	})
}

func TestNotificationKeyedColumns(t *testing.T) {
	// Keys and params are written together or not at all. A row carrying
	// title_key/body_key without the params they interpolate makes the client
	// resolve a frame it cannot fill — unfilled placeholders on screen — instead
	// of taking the clean English fallback.
	t.Run("keys and params are written when params marshal", func(t *testing.T) {
		c := NotificationContent{
			Title: "Review: ACP v2", TitleKey: "notifications.review_requested",
			Body: "alice requested your review", BodyKey: "notifications.review_requested.body",
			Params: map[string]any{"actor": "alice@example.com"},
		}
		titleKey, bodyKey, params := c.keyedColumns()
		if titleKey == nil || *titleKey != "notifications.review_requested" {
			t.Errorf("title_key = %v, want the key", titleKey)
		}
		if bodyKey == nil || *bodyKey != "notifications.review_requested.body" {
			t.Errorf("body_key = %v, want the key", bodyKey)
		}
		if len(params) == 0 {
			t.Error("params = empty, want the marshalled map")
		}
	})

	t.Run("a marshal failure clears both keys", func(t *testing.T) {
		c := NotificationContent{
			Title: "Review: ACP v2", TitleKey: "notifications.review_requested",
			BodyKey: "notifications.review_requested.body",
			Params:  map[string]any{"bad": func() {}},
		}
		titleKey, bodyKey, params := c.keyedColumns()
		if titleKey != nil || bodyKey != nil || params != nil {
			t.Fatalf("want an English-only row (all nil), got %v/%v/%q", titleKey, bodyKey, params)
		}
	})

	t.Run("an unkeyed row stays unkeyed", func(t *testing.T) {
		// Legacy call sites and agent notifications: no keys, no params, and the
		// columns must be SQL NULL rather than empty strings so the JSON
		// omitempty round-trip is unchanged.
		titleKey, bodyKey, params := NotificationContent{Title: "AI review escalated"}.keyedColumns()
		if titleKey != nil || bodyKey != nil || params != nil {
			t.Fatalf("want all nil, got %v/%v/%q", titleKey, bodyKey, params)
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
