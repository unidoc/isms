package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// recordedReq captures one outbound CLI API call.
type recordedReq struct {
	method string
	path   string
	body   map[string]interface{}
}

// cliServer spins an httptest server that records every request body and replies
// with a minimal JSON object the client can unmarshal. Returns the recorder.
func cliServer(t *testing.T) (*httptest.Server, *[]recordedReq) {
	t.Helper()
	var got []recordedReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		got = append(got, recordedReq{method: r.Method, path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"identifier":"X-001","title":"x"}`))
	}))
	t.Setenv("ISMS_API_URL", srv.URL)
	t.Setenv("ISMS_API_KEY", "test-token")
	return srv, &got
}

// refSet extracts the {type:[ids]} relations from a recorded create body.
func refSet(t *testing.T, body map[string]interface{}) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	refs, ok := body["references"].([]interface{})
	if !ok {
		return out
	}
	for _, r := range refs {
		m := r.(map[string]interface{})
		typ, _ := m["type"].(string)
		id, _ := m["id"].(string)
		out[typ] = append(out[typ], id)
	}
	return out
}

func TestBuildRefs(t *testing.T) {
	got := buildRefs(
		refSpec{"risk", []string{"RISK-1", " RISK-2 ", ""}},
		refSpec{"asset", []string{"A-1"}},
		refSpec{"system", nil},
	)
	want := []client_Reference{
		{"risk", "RISK-1"}, {"risk", "RISK-2"}, {"asset", "A-1"},
	}
	if !reflect.DeepEqual(toPlain(got), want) {
		t.Errorf("buildRefs = %+v, want %+v", toPlain(got), want)
	}
}

// client_Reference / toPlain decouple the assertion from the client package's
// import path while still comparing type+id.
type client_Reference struct{ Type, ID string }

func toPlain(refs interface{}) []client_Reference {
	v := reflect.ValueOf(refs)
	out := make([]client_Reference, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		e := v.Index(i)
		out = append(out, client_Reference{
			Type: e.FieldByName("Type").String(),
			ID:   e.FieldByName("ID").String(),
		})
	}
	return out
}

// #49: `audit complete` must send the (required) --summary, not silently drop it.
func TestAuditCompleteSendsSummary(t *testing.T) {
	_, got := cliServer(t)
	cmd := auditCmd()
	cmd.SetArgs([]string{"complete", "5", "--summary", "Quarterly audit closed"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(*got) == 0 {
		t.Fatal("no request sent")
	}
	last := (*got)[len(*got)-1]
	if last.body["summary"] != "Quarterly audit closed" {
		t.Errorf("summary not sent: body=%v", last.body)
	}
	if last.body["status"] != "completed" {
		t.Errorf("status not sent: body=%v", last.body)
	}
}

// #52: `incident add` must send its declared relation flags as references.
func TestIncidentAddSendsRelations(t *testing.T) {
	_, got := cliServer(t)
	cmd := incidentCmd()
	cmd.SetArgs([]string{"add", "--title", "Phish", "--description", "phishing wave",
		"--severity", "high",
		"--risks", "RISK-1,RISK-2", "--assets", "A-1", "--systems", "SYS-9"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	refs := refSet(t, (*got)[0].body)
	for typ, want := range map[string][]string{
		"risk": {"RISK-1", "RISK-2"}, "asset": {"A-1"}, "system": {"SYS-9"}} {
		if !reflect.DeepEqual(refs[typ], want) {
			t.Errorf("%s refs = %v, want %v (full=%v)", typ, refs[typ], want, refs)
		}
	}
}

// #52: `risk add` must send --assets and --documents as references.
func TestRiskAddSendsRelations(t *testing.T) {
	_, got := cliServer(t)
	cmd := riskCmd()
	cmd.SetArgs([]string{"add", "--title", "R", "--owner", "o@x.io",
		"--likelihood", "3", "--impact", "4",
		"--assets", "A-1,A-2", "--documents", "iso27001-5-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	refs := refSet(t, (*got)[0].body)
	if !reflect.DeepEqual(refs["asset"], []string{"A-1", "A-2"}) {
		t.Errorf("asset refs = %v (full=%v)", refs["asset"], refs)
	}
	if !reflect.DeepEqual(refs["document"], []string{"iso27001-5-1"}) {
		t.Errorf("document refs = %v (full=%v)", refs["document"], refs)
	}
}

// #52: `legal add` must send --documents as references.
func TestLegalAddSendsRelations(t *testing.T) {
	_, got := cliServer(t)
	cmd := legalCmd()
	cmd.SetArgs([]string{"add", "--title", "GDPR", "--documents", "iso27001-18-1,iso27001-18-2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	refs := refSet(t, (*got)[0].body)
	if !reflect.DeepEqual(refs["document"], []string{"iso27001-18-1", "iso27001-18-2"}) {
		t.Errorf("document refs = %v (full=%v)", refs["document"], refs)
	}
}
