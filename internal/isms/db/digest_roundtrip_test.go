package db

import (
	"context"
	"testing"
)

// Requires a migrated Postgres; skipped without ISMS_TEST_DATABASE_URL.

func TestDigestPreferenceRoundTrip(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	org, email := newTestOrgUser(t, d, "digest-pref")
	u, err := d.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AddOrgMember(ctx, org, u.ID, "reader"); err != nil {
		t.Fatal(err)
	}

	on, err := d.GetDigestEnabled(ctx, org, u.ID)
	if err != nil || !on {
		t.Fatalf("new member: enabled = %v, err = %v, want true", on, err)
	}
	if err := d.SetDigestEnabled(ctx, org, u.ID, false); err != nil {
		t.Fatal(err)
	}
	if on, _ := d.GetDigestEnabled(ctx, org, u.ID); on {
		t.Error("enabled after turning off")
	}
	out, err := d.ListDigestOptOuts(ctx, org)
	if err != nil || !out[u.ID] || len(out) != 1 {
		t.Errorf("opt-outs = %v, err = %v, want only user %d", out, err, u.ID)
	}
	if err := d.SetDigestEnabled(ctx, org, u.ID, true); err != nil {
		t.Fatal(err)
	}
	if on, _ := d.GetDigestEnabled(ctx, org, u.ID); !on {
		t.Error("not enabled after turning back on")
	}
	// AddOrgMember re-adding with a new role must not reset the preference.
	_ = d.SetDigestEnabled(ctx, org, u.ID, false)
	if err := d.AddOrgMember(ctx, org, u.ID, "manager"); err != nil {
		t.Fatal(err)
	}
	if on, _ := d.GetDigestEnabled(ctx, org, u.ID); on {
		t.Error("a role change reset the digest preference")
	}
	if err := d.SetDigestEnabled(ctx, org, u.ID+1000000, true); err == nil {
		t.Error("setting the preference of a non-member succeeded")
	}
}

func TestDigestClaim(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	org, email := newTestOrgUser(t, d, "digest-claim")
	u, err := d.GetUserByEmail(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	const week = "2026-W44"

	won, err := d.ClaimDigest(ctx, org, u.ID, week, "sent")
	if err != nil || !won {
		t.Fatalf("first claim: won = %v, err = %v", won, err)
	}
	if won, _ := d.ClaimDigest(ctx, org, u.ID, week, "sent"); won {
		t.Error("second claim for the same week won")
	}
	if won, _ := d.ClaimDigest(ctx, org, u.ID, week, "empty"); won {
		t.Error("a claim with another outcome won over an existing one")
	}
	if won, _ := d.ClaimDigest(ctx, org, u.ID, "2026-W45", "empty"); !won {
		t.Error("the next week's claim lost")
	}
	if got, err := d.ListDigestClaims(ctx, org, week); err != nil || !got[u.ID] || len(got) != 1 {
		t.Errorf("claims for %s = %v, err = %v", week, got, err)
	}
	if err := d.ReleaseDigestClaim(ctx, org, u.ID, week); err != nil {
		t.Fatal(err)
	}
	if won, _ := d.ClaimDigest(ctx, org, u.ID, week, "sent"); !won {
		t.Error("claim after release lost")
	}
	if _, err := d.ClaimDigest(ctx, org, u.ID, "2026-W46", "bogus"); err == nil {
		t.Error("an outcome outside sent/empty was accepted")
	}
}
