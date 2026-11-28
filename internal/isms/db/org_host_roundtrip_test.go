package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// hostTestOrg creates an org with an optional app domain, removed on cleanup.
func hostTestOrg(t *testing.T, d *DB, tag string, domain *string) *Organization {
	t.Helper()
	org := &Organization{
		Name:     "Host " + tag,
		Slug:     fmt.Sprintf("host-%s-%d", tag, time.Now().UnixNano()),
		RepoPath: "/tmp/none",
		Domain:   domain,
	}
	if err := d.CreateOrganization(context.Background(), org); err != nil {
		t.Fatalf("create %s: %v", tag, err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, org.ID)
	})
	return org
}

func sp(s string) *string { return &s }

// uniqueHost returns a host no other test run uses.
func uniqueHost(prefix string) string {
	return fmt.Sprintf("%s-%d.hosttest.example", prefix, time.Now().UnixNano())
}

func TestTrustDomainSetChangeClearAndLookup(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	appHost := uniqueHost("app")
	org := hostTestOrg(t, d, "a", sp("https://"+capitalize(appHost)+"/"))
	trust := uniqueHost("trust")

	if err := d.SetOrganizationTrustDomain(ctx, org.ID, &trust); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := d.GetOrganization(ctx, org.ID)
	if err != nil || got.TrustDomain == nil || *got.TrustDomain != trust {
		t.Fatalf("GetOrganization trust_domain = %v (%v), want %q", got.TrustDomain, err, trust)
	}
	for name, fn := range map[string]func() (*Organization, error){
		"slug": func() (*Organization, error) { return d.GetOrganizationBySlug(ctx, org.Slug) },
		"uuid": func() (*Organization, error) { return d.GetOrganizationByUUID(ctx, org.UUID) },
	} {
		if o, err := fn(); err != nil || o.TrustDomain == nil || *o.TrustDomain != trust {
			t.Errorf("%s lookup trust_domain not read: %v %v", name, o, err)
		}
	}
	list, err := d.ListOrganizations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range list {
		if o.ID == org.ID {
			found = o.TrustDomain != nil && *o.TrustDomain == trust
		}
	}
	if !found {
		t.Error("ListOrganizations did not read trust_domain")
	}

	o, kind, err := d.GetOrganizationByHost(ctx, trust)
	if err != nil || o.ID != org.ID || kind != OrgHostTrust {
		t.Fatalf("by trust host = %v %v %v, want org %d OrgHostTrust", o, kind, err, org.ID)
	}
	o, kind, err = d.GetOrganizationByHost(ctx, appHost)
	if err != nil || o.ID != org.ID || kind != OrgHostApp {
		t.Fatalf("by app host (stored with scheme, slash, case) = %v %v %v, want OrgHostApp", o, kind, err)
	}
	if _, _, err := d.GetOrganizationByHost(ctx, uniqueHost("nobody")); err == nil {
		t.Error("unknown host matched an organization")
	}
	if o, err := d.GetOrganizationByDomain(ctx, capitalize(appHost)); err != nil || o.ID != org.ID {
		t.Errorf("GetOrganizationByDomain is not case-insensitive: %v %v", o, err)
	}

	changed := uniqueHost("trust2")
	if err := d.SetOrganizationTrustDomain(ctx, org.ID, &changed); err != nil {
		t.Fatalf("change: %v", err)
	}
	if _, _, err := d.GetOrganizationByHost(ctx, trust); err == nil {
		t.Error("old trust host still resolves")
	}
	if o, _, err := d.GetOrganizationByHost(ctx, changed); err != nil || o.ID != org.ID {
		t.Errorf("new trust host does not resolve: %v %v", o, err)
	}

	if err := d.SetOrganizationTrustDomain(ctx, org.ID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	got, _ = d.GetOrganization(ctx, org.ID)
	if got.TrustDomain != nil {
		t.Errorf("trust_domain after clear = %q, want nil", *got.TrustDomain)
	}
	if _, _, err := d.GetOrganizationByHost(ctx, changed); err == nil {
		t.Error("cleared trust host still resolves")
	}
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}

func TestTrustDomainSetUnknownOrg(t *testing.T) {
	d := testDB(t)
	h := uniqueHost("x")
	if err := d.SetOrganizationTrustDomain(context.Background(), -1, &h); err == nil || errors.Is(err, ErrOrgHostTaken) {
		t.Fatalf("unknown org err = %v, want a not-found error", err)
	}
}

func TestOrgHostCollisionsAreRefused(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	appHost := uniqueHost("app")
	trustHost := uniqueHost("trust")
	a := hostTestOrg(t, d, "a", nil)
	b := hostTestOrg(t, d, "b", sp("https://"+appHost+"/"))
	if err := d.SetOrganizationTrustDomain(ctx, a.ID, &trustHost); err != nil {
		t.Fatal(err)
	}

	t.Run("trust equals another org's trust, other case", func(t *testing.T) {
		up := capitalize(trustHost)
		if err := d.SetOrganizationTrustDomain(ctx, b.ID, &up); !errors.Is(err, ErrOrgHostTaken) {
			t.Fatalf("err = %v, want ErrOrgHostTaken", err)
		}
	})
	t.Run("trust equals another org's app domain stored with scheme", func(t *testing.T) {
		if err := d.SetOrganizationTrustDomain(ctx, a.ID, &appHost); !errors.Is(err, ErrOrgHostTaken) {
			t.Fatalf("err = %v, want ErrOrgHostTaken", err)
		}
	})
	t.Run("trust equals own app domain", func(t *testing.T) {
		if err := d.SetOrganizationTrustDomain(ctx, b.ID, &appHost); !errors.Is(err, ErrOrgHostTaken) {
			t.Fatalf("err = %v, want ErrOrgHostTaken", err)
		}
	})
	t.Run("app domain equals an existing trust domain (create)", func(t *testing.T) {
		c := &Organization{Name: "C", Slug: fmt.Sprintf("host-c-%d", time.Now().UnixNano()), RepoPath: "/tmp/none", Domain: sp("HTTPS://" + trustHost + "/")}
		err := d.CreateOrganization(ctx, c)
		if !errors.Is(err, ErrOrgHostTaken) {
			_, _ = d.pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, c.ID)
			t.Fatalf("err = %v, want ErrOrgHostTaken", err)
		}
	})
	t.Run("app domain updated to an existing trust domain", func(t *testing.T) {
		_, err := d.pool.Exec(ctx, `UPDATE organizations SET domain = $2 WHERE id = $1`, b.ID, trustHost)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("err = %v, want 23505", err)
		}
	})
}

func TestOrgHostSoftDelete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	trustHost := uniqueHost("trust")
	appHost := uniqueHost("app")
	a := hostTestOrg(t, d, "a", sp(appHost))
	if err := d.SetOrganizationTrustDomain(ctx, a.ID, &trustHost); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteOrganization(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.GetOrganizationByHost(ctx, trustHost); err == nil {
		t.Error("deleted org's trust host still resolves")
	}

	b := hostTestOrg(t, d, "b", nil)
	if err := d.SetOrganizationTrustDomain(ctx, b.ID, &trustHost); err != nil {
		t.Fatalf("a deleted org's trust host blocked a live org: %v", err)
	}
	c := hostTestOrg(t, d, "c", sp(appHost))
	_ = c

	// Restoring the deleted org while its hosts are taken again is refused.
	_, err := d.pool.Exec(ctx, `UPDATE organizations SET deleted_at = NULL WHERE id = $1`, a.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("restore err = %v, want 23505", err)
	}
}

func TestOrgHostUnrelatedUpdatesPass(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	trustHost := uniqueHost("trust")
	a := hostTestOrg(t, d, "a", sp(uniqueHost("app")))
	if err := d.SetOrganizationTrustDomain(ctx, a.ID, &trustHost); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(ctx, `UPDATE organizations SET name = 'Renamed' WHERE id = $1`, a.ID); err != nil {
		t.Fatalf("unrelated update: %v", err)
	}
	if err := d.SetOrganizationTrustDomain(ctx, a.ID, &trustHost); err != nil {
		t.Fatalf("setting the same trust domain again: %v", err)
	}
}

func TestTrustDomainCheckRefusesNonCanonicalValues(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	a := hostTestOrg(t, d, "a", nil)
	for _, bad := range []string{"Trust.Example.com", "https://trust.example.com", "trust", "trust.example.com:8443", "trust.example.com/"} {
		_, err := d.pool.Exec(ctx, `UPDATE organizations SET trust_domain = $2 WHERE id = $1`, a.ID, bad)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("trust_domain %q: err = %v, want check violation 23514", bad, err)
		}
	}
}

func TestV100MigrationReRunKeepsTrustDomains(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "migrations", "*_v1.0.0.sql"))
	if err != nil || len(files) != 1 {
		t.Fatalf("want exactly one *_v1.0.0.sql, got %v (%v)", files, err)
	}
	sqlText, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	a := hostTestOrg(t, d, "a", sp(uniqueHost("app")))
	trustHost := uniqueHost("trust")
	if err := d.SetOrganizationTrustDomain(ctx, a.ID, &trustHost); err != nil {
		t.Fatal(err)
	}
	if _, err := d.pool.Exec(ctx, string(sqlText)); err != nil {
		t.Fatalf("re-running the migration: %v", err)
	}
	got, err := d.GetOrganization(ctx, a.ID)
	if err != nil || got.TrustDomain == nil || *got.TrustDomain != trustHost {
		t.Fatalf("trust domain after re-run = %v (%v)", got.TrustDomain, err)
	}
	other := hostTestOrg(t, d, "b", nil)
	if err := d.SetOrganizationTrustDomain(ctx, other.ID, &trustHost); !errors.Is(err, ErrOrgHostTaken) {
		t.Fatalf("after re-run err = %v, want ErrOrgHostTaken", err)
	}
}
