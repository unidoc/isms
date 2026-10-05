package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// #385: `legal update`, `incident update` and `corrective update` sent a
// whole db record, so every unset flag reached the server as "", 0 or false
// and tripped a CHECK constraint. Each must send exactly the flags given.

func TestUpdateCommandsSendOnlyTheFlagsGiven(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		path string
		want map[string]interface{}
	}{
		{
			"legal update notes", legalCmd,
			[]string{"update", "3", "--notes", "new notes"},
			"/v1/legal/3",
			map[string]interface{}{"notes": "new notes"},
		},
		{
			"legal update scores and date", legalCmd,
			[]string{"update", "3", "--likelihood", "4", "--target-impact", "2", "--next-review", "2027-01-31"},
			"/v1/legal/3",
			map[string]interface{}{"current_likelihood": float64(4), "target_impact": float64(2), "next_review": float64(1801353600)},
		},
		{
			"incident update notes", incidentCmd,
			[]string{"update", "4", "--notes", "new notes"},
			"/v1/incidents/4",
			map[string]interface{}{"notes": "new notes"},
		},
		{
			"incident update severity and CIA", incidentCmd,
			[]string{"update", "4", "--severity", "high", "--affects-c"},
			"/v1/incidents/4",
			map[string]interface{}{"severity": "high", "affects_c": true},
		},
		{
			"corrective update notes", correctiveCmd,
			[]string{"update", "5", "--notes", "new notes"},
			"/v1/corrective-actions/5",
			map[string]interface{}{"notes": "new notes"},
		},
		{
			"corrective update root cause", correctiveCmd,
			[]string{"update", "5", "--root-cause", "rc"},
			"/v1/corrective-actions/5",
			map[string]interface{}{"root_cause": "rc"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := cliServer(t)
			defer srv.Close()
			cmd := tc.cmd()
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(*got) != 1 {
				t.Fatalf("want exactly one API call, got %d", len(*got))
			}
			r := (*got)[0]
			if r.method != "PUT" || r.path != tc.path {
				t.Errorf("hit %s %s, want PUT %s", r.method, r.path, tc.path)
			}
			if !reflect.DeepEqual(r.body, tc.want) {
				t.Errorf("body = %v, want %v", r.body, tc.want)
			}
		})
	}
}

// An explicitly given zero or false is a real value and must be sent, not
// dropped by omitempty.
func TestUpdateCommandsSendExplicitZeroValues(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
		want map[string]interface{}
	}{
		{"legal completion 0", legalCmd, []string{"update", "3", "--completion", "0"},
			map[string]interface{}{"completion": float64(0)}},
		{"incident data-breach=false", incidentCmd, []string{"update", "4", "--data-breach=false"},
			map[string]interface{}{"data_breach": false}},
		{"corrective empty notes", correctiveCmd, []string{"update", "5", "--notes", ""},
			map[string]interface{}{"notes": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := cliServer(t)
			defer srv.Close()
			cmd := tc.cmd()
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(*got) != 1 {
				t.Fatalf("want exactly one API call, got %d", len(*got))
			}
			if !reflect.DeepEqual((*got)[0].body, tc.want) {
				t.Errorf("body = %v, want %v", (*got)[0].body, tc.want)
			}
		})
	}
}

// A date flag given as "" asks to clear the date, so it must go on the wire
// as null. Omitting it left the date untouched while the command still
// printed success.
func TestLegalUpdateSendsEmptyDateAsNull(t *testing.T) {
	for _, flag := range []string{"last-review", "next-review"} {
		t.Run(flag, func(t *testing.T) {
			srv, got := cliServer(t)
			defer srv.Close()
			cmd := legalCmd()
			cmd.SetArgs([]string{"update", "3", "--" + flag, ""})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(*got) != 1 {
				t.Fatalf("want exactly one API call, got %d", len(*got))
			}
			key := strings.ReplaceAll(flag, "-", "_")
			want := map[string]interface{}{key: nil}
			if !reflect.DeepEqual((*got)[0].body, want) {
				t.Errorf("body = %v, want %v", (*got)[0].body, want)
			}
		})
	}
}

// `incident resolve --root-cause` and `incident close --lessons` changed the
// status, then saved the text in a second request whose error was discarded.
// They now send one status request that carries the text.
func TestIncidentResolveAndCloseSendTextWithStatus(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want map[string]interface{}
	}{
		{"resolve with root cause", []string{"resolve", "4", "--root-cause", "expired cert"},
			map[string]interface{}{"status": "resolved", "root_cause": "expired cert"}},
		{"resolve without root cause", []string{"resolve", "4"},
			map[string]interface{}{"status": "resolved"}},
		{"close with lessons", []string{"close", "4", "--lessons", "monitor expiry"},
			map[string]interface{}{"status": "closed", "lessons_learned": "monitor expiry"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := cliServer(t)
			defer srv.Close()
			cmd := incidentCmd()
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(*got) != 1 {
				t.Fatalf("want exactly one API call, got %d", len(*got))
			}
			r := (*got)[0]
			if r.method != "PUT" || r.path != "/v1/incidents/4/status" {
				t.Errorf("hit %s %s, want PUT /v1/incidents/4/status", r.method, r.path)
			}
			if !reflect.DeepEqual(r.body, tc.want) {
				t.Errorf("body = %v, want %v", r.body, tc.want)
			}
		})
	}
}

// A rejected request must fail the command, never print success.
func TestIncidentResolveAndCloseReportServerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"rejected"}`))
	}))
	defer srv.Close()
	t.Setenv("ISMS_API_URL", srv.URL)
	t.Setenv("ISMS_API_KEY", "test-token")

	for _, args := range [][]string{
		{"resolve", "4", "--root-cause", "rc"},
		{"close", "4", "--lessons", "l"},
	} {
		cmd := incidentCmd()
		cmd.SetArgs(args)
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		if err := cmd.Execute(); err == nil {
			t.Errorf("incident %s: want an error from a 400, got success", args[0])
		}
	}
}

// --risks and --documents were accepted by the update commands and silently
// ignored; the update API takes no references. They are now unknown flags.
func TestUpdateCommandsRejectIgnoredLinkFlags(t *testing.T) {
	cases := []struct {
		cmd  func() *cobra.Command
		args []string
	}{
		{incidentCmd, []string{"update", "4", "--risks", "RISK-1"}},
		{legalCmd, []string{"update", "3", "--documents", "doc-1"}},
	}
	for _, tc := range cases {
		srv, got := cliServer(t)
		cmd := tc.cmd()
		cmd.SetArgs(tc.args)
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		if err := cmd.Execute(); err == nil {
			t.Errorf("%v: want an unknown-flag error, got success", tc.args)
		}
		if len(*got) != 0 {
			t.Errorf("%v: want no API call, got %d", tc.args, len(*got))
		}
		srv.Close()
	}
}
