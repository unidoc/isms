package main

import (
	"reflect"
	"testing"
)

// #414: `risk treat` always sent a status — "accepted" for --decision accept,
// but "treating" for mitigate/transfer/avoid. "treating" was never a real
// status (db.RiskStatuses only ever allowed draft/open/closed, now also
// accepted), so every non-accept decision 400'd on the invalid status before
// the treatment field could ever be saved. Only an accept decision should
// touch status at all; everything else must leave the risk's status alone and
// write only the treatment.
func TestRiskTreatSendsStatusOnlyOnAccept(t *testing.T) {
	cases := []struct {
		name     string
		decision string
		want     map[string]interface{}
	}{
		{"accept sets status", "accept", map[string]interface{}{"treatment": "accept", "status": "accepted"}},
		{"mitigate leaves status alone", "mitigate", map[string]interface{}{"treatment": "mitigate"}},
		{"transfer leaves status alone", "transfer", map[string]interface{}{"treatment": "transfer"}},
		{"avoid leaves status alone", "avoid", map[string]interface{}{"treatment": "avoid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := cliServer(t)
			defer srv.Close()
			cmd := riskTreatCmd()
			cmd.SetArgs([]string{"RISK-20", "--decision", tc.decision})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if len(*got) != 1 {
				t.Fatalf("want exactly one API call, got %d", len(*got))
			}
			r := (*got)[0]
			if r.method != "PUT" || r.path != "/v1/risks/RISK-20" {
				t.Errorf("hit %s %s, want PUT /v1/risks/RISK-20", r.method, r.path)
			}
			if !reflect.DeepEqual(r.body, tc.want) {
				t.Errorf("body = %v, want %v", r.body, tc.want)
			}
		})
	}
}
