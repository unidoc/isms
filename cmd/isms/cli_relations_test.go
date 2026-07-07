package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"isms.sh/internal/isms/client"
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
	want := []client.Reference{
		{Type: "risk", ID: "RISK-1"}, {Type: "risk", ID: "RISK-2"}, {Type: "asset", ID: "A-1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildRefs = %+v, want %+v", got, want)
	}
}

// #49: `audit complete` must send the (required) --summary, not silently drop it.
func TestAuditCompleteSendsSummary(t *testing.T) {
	srv, got := cliServer(t)
	defer srv.Close()
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
	srv, got := cliServer(t)
	defer srv.Close()
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
	srv, got := cliServer(t)
	defer srv.Close()
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
	srv, got := cliServer(t)
	defer srv.Close()
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

// #51: `review approve` must go through the dedicated approval handler
// (POST /reviews/:id/approve), not PUT /reviews/:id/status (which the server
// rejects for anything but "closed") — and must not side-step it via /approvals.
func TestReviewApproveUsesApprovalEndpoint(t *testing.T) {
	srv, got := cliServer(t)
	defer srv.Close()
	cmd := reviewCmd()
	cmd.SetArgs([]string{"approve", "7", "--comment", "LGTM"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("expected exactly one request, got %d: %+v", len(*got), *got)
	}
	r := (*got)[0]
	if r.method != "POST" || r.path != "/v1/reviews/7/approve" {
		t.Errorf("approve hit %s %s, want POST /v1/reviews/7/approve", r.method, r.path)
	}
	if r.body["decision"] != "approved" {
		t.Errorf("decision not sent as 'approved': body=%v", r.body)
	}
	if r.body["comment"] != "LGTM" {
		t.Errorf("comment not forwarded: body=%v", r.body)
	}
}

func TestSupplierAddSendsCIA(t *testing.T) {
	srv, got := cliServer(t)
	defer srv.Close()
	cmd := supplierCmd()
	cmd.SetArgs([]string{"add", "--name", "Acme", "--type", "saas", "--criticality", "high",
		"--confidentiality", "3", "--integrity", "4", "--availability", "5"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	b := (*got)[len(*got)-1].body
	if b["confidentiality"] != float64(3) || b["integrity"] != float64(4) || b["availability"] != float64(5) {
		t.Errorf("CIA ratings not sent: %+v", b)
	}
}

func TestSupplierAddOmitsUnsetCIA(t *testing.T) {
	srv, got := cliServer(t)
	defer srv.Close()
	cmd := supplierCmd()
	cmd.SetArgs([]string{"add", "--name", "Acme", "--type", "saas", "--criticality", "high"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Unset → null (not assessed); the **int update DTO treats null as skip.
	if b := (*got)[len(*got)-1].body; b["confidentiality"] != nil {
		t.Errorf("unset confidentiality should be null, got %v", b["confidentiality"])
	}
}

func TestSupplierAddRejectsOutOfRangeCIA(t *testing.T) {
	srv, _ := cliServer(t)
	defer srv.Close()
	cmd := supplierCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"add", "--name", "Acme", "--type", "saas", "--criticality", "high",
		"--confidentiality", "9"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected an error for --confidentiality 9 (out of 0-5)")
	}
}

func TestSupplierAddRejectsNegativeCIA(t *testing.T) {
	srv, _ := cliServer(t)
	defer srv.Close()
	cmd := supplierCmd()
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"add", "--name", "Acme", "--type", "saas", "--criticality", "high",
		"--confidentiality", "-1"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected an error for --confidentiality -1 (out of 0-5)")
	}
}
