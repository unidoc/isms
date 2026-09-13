package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// #213/#216: `isms risk add` must be able to supply required custom fields via
// --field key=value, and must fail fast client-side (no API call) when a
// required field is missing, rather than forwarding the server's generic 400.

func riskCustomFieldsServer(t *testing.T) (*httptest.Server, *[]recordedReq) {
	t.Helper()
	var got []recordedReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/risks/custom-fields" {
			_, _ = w.Write([]byte(`[{"key":"severity","label":"Severity","type":"select","required":true,"options":["Low","High"]}]`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		got = append(got, recordedReq{method: r.Method, path: r.URL.Path, body: body})
		_, _ = w.Write([]byte(`{"id":1,"identifier":"X-001","title":"x"}`))
	}))
	t.Setenv("ISMS_API_URL", srv.URL)
	t.Setenv("ISMS_API_KEY", "test-token")
	return srv, &got
}

func TestRiskAddMissingRequiredCustomFieldFailsClientSide(t *testing.T) {
	srv, got := riskCustomFieldsServer(t)
	defer srv.Close()
	cmd := riskCmd()
	cmd.SetArgs([]string{"add", "--title", "R", "--owner", "o@x.io",
		"--likelihood", "3", "--impact", "4"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for missing required custom field")
	}
	if len(*got) != 0 {
		t.Errorf("no create call should have been made, got %d requests", len(*got))
	}
}

func TestRiskAddWithRequiredCustomFieldSucceeds(t *testing.T) {
	srv, got := riskCustomFieldsServer(t)
	defer srv.Close()
	cmd := riskCmd()
	cmd.SetArgs([]string{"add", "--title", "R", "--owner", "o@x.io",
		"--likelihood", "3", "--impact", "4",
		"--field", "severity=Low"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(*got) == 0 {
		t.Fatal("expected the create call to be made")
	}
	body := (*got)[0].body
	cf, _ := body["custom_fields"].(map[string]interface{})
	if cf["severity"] != "Low" {
		t.Errorf("custom_fields.severity = %v, want Low (full body=%v)", cf["severity"], body)
	}
}
