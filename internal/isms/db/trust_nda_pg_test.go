package db

import (
	"context"
	"testing"
)

// Text that is not valid UTF-8 must not fail an acceptance (Postgres refuses it
// in a TEXT column): it is cleaned, and the address is capped.
func TestRecordTrustNDAAcceptanceCleansUserAgentAndIP(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "nda-utf8")
	var visitorID int64
	if err := d.pool.QueryRow(ctx, `INSERT INTO trust_visitors (organization_id, email) VALUES ($1, 'utf8@example.test') RETURNING id`, orgID).Scan(&visitorID); err != nil {
		t.Fatal(err)
	}
	long := ""
	for i := 0; i < 100; i++ {
		long += "1234567890"
	}
	for i, c := range []struct{ method, ua, ip, wantUA, wantIP string }{
		{TrustNDAAccepted, "Moz\xffilla/5.0", "10.0.0.\xfe1", "Mozilla/5.0", "10.0.0.1"},
		{TrustNDADomainSkip, "caf\xc3", long, "caf", long[:maxNDAIP]},
	} {
		in, err := d.RecordTrustNDAAcceptance(ctx, orgID, &TrustNDAAcceptance{
			VisitorID: visitorID, DocumentID: "nda", VersionID: 1, Version: "1", ContentHash: "h" + string(rune('a'+i)),
			CommitHash: "abc", FilePath: "documents/nda.md", Method: c.method, UserAgent: c.ua, IP: c.ip,
		})
		if err != nil || !in {
			t.Fatalf("case %d: inserted %v, err %v", i, in, err)
		}
		var ua, ip, commit, path string
		if err := d.pool.QueryRow(ctx, `SELECT user_agent, ip, commit_hash, file_path FROM trust_nda_acceptances
			WHERE organization_id = $1 AND visitor_id = $2 AND method = $3`, orgID, visitorID, c.method).Scan(&ua, &ip, &commit, &path); err != nil {
			t.Fatal(err)
		}
		if ua != c.wantUA || ip != c.wantIP || commit != "abc" || path != "documents/nda.md" {
			t.Errorf("case %d: stored ua %q ip %q commit %q path %q", i, ua, ip, commit, path)
		}
	}
}

func TestClampText(t *testing.T) {
	for _, c := range []struct {
		in   string
		max  int
		want string
	}{
		{"abc", 10, "abc"}, {"a\xffb", 10, "ab"}, {"abcdef", 3, "abc"}, {"ééé", 3, "é"}, {"", 5, ""},
	} {
		if got := clampText(c.in, c.max); got != c.want {
			t.Errorf("clampText(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}
