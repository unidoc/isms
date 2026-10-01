package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// nullClearsKeys are the update-request fields where a null now clears the
// stored value (every optional date, link and number of the 13 update DTOs).
var nullClearsKeys = []string{
	"confidentiality", "integrity", "availability", "last_review", "next_review",
	"contract_expiry", "supplier_id", "current_likelihood", "current_impact",
	"confidentiality_impact", "integrity_impact", "availability_impact",
	"inherent_likelihood", "inherent_impact", "inherent_confidentiality_impact",
	"inherent_integrity_impact", "inherent_availability_impact",
	"target_likelihood", "target_impact", "treatment_due_date", "due_date",
	"authority_notified_at", "subjects_notified_at", "planned_date", "end_date",
	"recurrence_days", "planned_at", "target_value", "window_seconds",
	"started_at", "success", "value_numeric",
}

// #381: an explicit null in an update body now clears the field, so an edit
// command must never put a key on the wire that the user did not ask to
// change. Marshalling a bare db.Risk or db.System writes every nullable field
// without omitempty as null, which would blank the assessment and the CIA
// ratings of the record being edited.

func assertNoNullKeys(t *testing.T, got []recordedReq) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want exactly one API call, got %d", len(got))
	}
	for _, k := range nullClearsKeys {
		if v, ok := got[0].body[k]; ok && v == nil {
			t.Errorf("%s %s sends %q as null, which now clears it", got[0].method, got[0].path, k)
		}
	}
}

func TestRiskAssessSendsOnlyTheFlagsGiven(t *testing.T) {
	srv, got := cliServer(t)
	defer srv.Close()
	cmd := riskCmd()
	cmd.SetArgs([]string{"assess", "RISK-1", "--likelihood", "3"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	assertNoNullKeys(t, *got)
	body := (*got)[0].body
	if body["current_likelihood"] != float64(3) {
		t.Errorf("current_likelihood = %v, want 3", body["current_likelihood"])
	}
	if _, ok := body["current_impact"]; ok {
		t.Errorf("current_impact must be omitted when --impact is not given")
	}
}

func TestRiskTreatSendsNoNullKeys(t *testing.T) {
	srv, got := cliServer(t)
	defer srv.Close()
	cmd := riskCmd()
	cmd.SetArgs([]string{"treat", "RISK-1", "--decision", "mitigate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	assertNoNullKeys(t, *got)
	if (*got)[0].body["treatment"] != "mitigate" {
		t.Errorf("treatment = %v, want mitigate", (*got)[0].body["treatment"])
	}
}

func TestSystemEditSendsOnlyTheFlagsGiven(t *testing.T) {
	srv, got := cliServer(t)
	defer srv.Close()
	cmd := systemCmd()
	cmd.SetArgs([]string{"edit", "SYS-1", "--name", "Renamed"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	assertNoNullKeys(t, *got)
	body := (*got)[0].body
	if body["name"] != "Renamed" {
		t.Errorf("name = %v, want Renamed", body["name"])
	}
	for _, k := range []string{"confidentiality", "integrity", "availability", "supplier_id"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s must be omitted when its flag is not given", k)
		}
	}
}

// TestOtherUpdateCommandsSendNoNullKeys covers the edit commands that already
// marshal a whole record: their date and number fields carry omitempty or a
// concrete value, so none of them may put a cleared key on the wire.
func TestOtherUpdateCommandsSendNoNullKeys(t *testing.T) {
	cases := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{"legal update", legalCmd, []string{"update", "1", "--title", "GDPR"}},
		{"incident update", incidentCmd, []string{"update", "1", "--assignee", "a@x.io"}},
		{"incident resolve", incidentCmd, []string{"resolve", "1", "--root-cause", "rc"}},
		{"incident close", incidentCmd, []string{"close", "1", "--lessons", "l"}},
		{"corrective update", correctiveCmd, []string{"update", "1", "--root-cause", "rc"}},
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
			// resolve and close send a status call first, then the update.
			last := (*got)[len(*got)-1:]
			assertNoNullKeys(t, last)
		})
	}
}
