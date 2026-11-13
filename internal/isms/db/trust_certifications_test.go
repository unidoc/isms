package db

// Round-trip tests for trust center certifications (#521). They run only when
// ISMS_TEST_DATABASE_URL points at a migrated database. Every date is chosen by
// Postgres (CURRENT_DATE + n), never from Go's clock.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const certActor = "mgr@example.com"

// pgDay returns CURRENT_DATE + n as YYYY-MM-DD, computed by Postgres.
func pgDay(t *testing.T, d *DB, n int) string {
	t.Helper()
	var s string
	if err := d.pool.QueryRow(context.Background(),
		`SELECT to_char(CURRENT_DATE + $1::int, 'YYYY-MM-DD')`, n).Scan(&s); err != nil {
		t.Fatalf("pgDay: %v", err)
	}
	return s
}

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func strp(s string) *string { return &s }

func certFile(tag string) *TrustCertificationFile {
	return &TrustCertificationFile{
		Key: "org/trust/certifications/" + tag + ".pdf", Name: tag + ".pdf",
		ContentType: "application/pdf", Size: 1234,
		SHA256: strings.Repeat("ab", 32),
	}
}

func linkCertInput(label string) TrustCertificationInput {
	return TrustCertificationInput{Label: label, LinkType: "certificate", Issuer: "BSI", URL: "https://example.com/cert"}
}

func mustCreateCert(t *testing.T, d *DB, orgID int, in TrustCertificationInput) *TrustCertification {
	t.Helper()
	c, err := d.CreateTrustCertification(context.Background(), orgID, in, certActor)
	if err != nil {
		t.Fatalf("create certification %q: %v", in.Label, err)
	}
	return c
}

func certChangelog(t *testing.T, d *DB, orgID int, id int64) []string {
	t.Helper()
	rows, err := d.pool.Query(context.Background(), `
		SELECT action || '|' || COALESCE(field,'') || '|' || COALESCE(old_value,'<nil>') || '|' || COALESCE(new_value,'<nil>')
		FROM entity_changelog
		WHERE organization_id = $1 AND entity_type = 'trust_center_certification' AND entity_id = $2
		ORDER BY id`, orgID, id)
	if err != nil {
		t.Fatalf("changelog: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestParseTrustDate(t *testing.T) {
	good := []string{"2026-11-13", "2024-02-29", "0001-01-01"}
	for _, s := range good {
		got, err := ParseTrustDate(s)
		if err != nil || got != s {
			t.Errorf("ParseTrustDate(%q) = %q, %v; want ok", s, got, err)
		}
	}
	bad := []string{
		"", " ", "2026-13-01", "2026-02-30", "2023-02-29", "2026-1-5", "2026-11-3",
		"2026-11-13T00:00:00Z", "2026-11-13T10:00:00+02:00", "0000-01-01", "0000-12-31", "2026-11-13 ", " 2026-11-13",
		"20261113", "13/11/2026", "1794000000", "null", "2026-11-13Z",
	}
	for _, s := range bad {
		if got, err := ParseTrustDate(s); !errors.Is(err, ErrTrustCenterInvalid) {
			t.Errorf("ParseTrustDate(%q) = %q, %v; want ErrTrustCenterInvalid", s, got, err)
		}
	}
}

func TestParseOptionalTrustDate(t *testing.T) {
	if got, err := ParseOptionalTrustDate(nil); got != nil || err != nil {
		t.Errorf("nil = %v, %v; want nil, nil (null clears)", got, err)
	}
	if _, err := ParseOptionalTrustDate(strp("")); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("empty string err = %v, want ErrTrustCenterInvalid (empty is not null)", err)
	}
	got, err := ParseOptionalTrustDate(strp("2026-11-13"))
	if err != nil || got == nil || *got != "2026-11-13" {
		t.Errorf("valid = %v, %v", got, err)
	}
}

func TestTrustCertificationCreateFileAndLink(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certc")

	from, until := pgDay(t, d, -100), pgDay(t, d, 200)
	fc := mustCreateCert(t, d, orgID, TrustCertificationInput{
		Label: "ISO 27001", LinkType: "certificate", Issuer: "BSI",
		ValidFrom: &from, ValidUntil: &until, File: certFile("iso"),
	})
	if fc.ID == 0 || fc.FileKey == "" || fc.FileName != "iso.pdf" || fc.FileContentType != "application/pdf" ||
		fc.FileSize != 1234 || fc.URL != "" || fc.State != "current" || fc.Item != nil || fc.CreatedBy != certActor {
		t.Errorf("file certification = %+v", fc)
	}
	if fc.ValidFrom == nil || *fc.ValidFrom != from || fc.ValidUntil == nil || *fc.ValidUntil != until {
		t.Errorf("dates = %v..%v, want %s..%s", fc.ValidFrom, fc.ValidUntil, from, until)
	}

	lc := mustCreateCert(t, d, orgID, linkCertInput("SOC 2"))
	if lc.URL != "https://example.com/cert" || lc.FileKey != "" || lc.ValidUntil != nil || lc.State != "" {
		t.Errorf("link certification = %+v", lc)
	}

	got, err := d.GetTrustCertification(ctx, orgID, fc.ID)
	if err != nil || got.Label != "ISO 27001" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if cl := certChangelog(t, d, orgID, fc.ID); len(cl) != 1 || cl[0] != "create||<nil>|ISO 27001" {
		t.Errorf("create changelog = %v", cl)
	}

	// Dates round-trip as the exact day in JSON too.
	b, _ := marshalJSON(fc)
	if !strings.Contains(b, `"valid_until":"`+until+`"`) {
		t.Errorf("json %s lacks plain valid_until %s", b, until)
	}
	b, _ = marshalJSON(lc)
	if strings.Contains(b, "valid_until") {
		t.Errorf("json %s should omit a null valid_until", b)
	}
	if strings.Contains(b, "file_key") {
		t.Errorf("json %s leaks the storage key", b)
	}
}

func TestTrustCertificationCreateValidation(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certv")
	a, b := pgDay(t, d, 10), pgDay(t, d, 5)

	cases := map[string]TrustCertificationInput{
		"contract type": {Label: "x", LinkType: "contract", URL: "https://e.com"},
		"unknown type":  {Label: "x", LinkType: "nope", URL: "https://e.com"},
		"both":          {Label: "x", LinkType: "other", URL: "https://e.com", File: certFile("both")},
		"neither":       {Label: "x", LinkType: "other"},
		"from > until":  {Label: "x", LinkType: "other", URL: "https://e.com", ValidFrom: &a, ValidUntil: &b},
		"label empty":   {Label: "", LinkType: "other", URL: "https://e.com"},
		"label blank":   {Label: "   ", LinkType: "other", URL: "https://e.com"},
		"label 201":     {Label: strings.Repeat("a", 201), LinkType: "other", URL: "https://e.com"},
		"issuer 201":    {Label: "x", LinkType: "other", URL: "https://e.com", Issuer: strings.Repeat("a", 201)},
		"url 2049":      {Label: "x", LinkType: "other", URL: "https://e.com/" + strings.Repeat("a", 2040)},
		"bad date":      {Label: "x", LinkType: "other", URL: "https://e.com", ValidUntil: strp("2026-13-01")},
		"rfc3339":       {Label: "x", LinkType: "other", URL: "https://e.com", ValidFrom: strp("2026-11-13T00:00:00Z")},
		"file type":     {Label: "x", LinkType: "other", File: &TrustCertificationFile{Key: "k", Name: "a.html", ContentType: "text/html", Size: 1, SHA256: "s"}},
		"file no key":   {Label: "x", LinkType: "other", File: &TrustCertificationFile{Name: "a.pdf", ContentType: "application/pdf", Size: 1, SHA256: "s"}},
	}
	for name, in := range cases {
		if _, err := d.CreateTrustCertification(ctx, orgID, in, certActor); !errors.Is(err, ErrTrustCenterInvalid) {
			t.Errorf("%s: err = %v, want ErrTrustCenterInvalid", name, err)
		}
	}
	// Boundaries that must pass: 200-rune label (multi-byte), 200-char issuer, equal dates.
	same := pgDay(t, d, 3)
	ok := TrustCertificationInput{Label: strings.Repeat("é", 200), LinkType: "attestation", URL: "https://e.com",
		Issuer: strings.Repeat("i", 200), ValidFrom: &same, ValidUntil: &same}
	if _, err := d.CreateTrustCertification(ctx, orgID, ok, certActor); err != nil {
		t.Errorf("boundary input rejected: %v", err)
	}
	if _, err := d.CreateTrustCertification(ctx, orgID, linkCertInput("x"), ""); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("empty actor err = %v, want ErrTrustCenterInvalid", err)
	}
	for _, typ := range []string{"certificate", "audit_report", "penetration_test", "attestation", "other"} {
		in := linkCertInput("t-" + typ)
		in.LinkType = typ
		if _, err := d.CreateTrustCertification(ctx, orgID, in, certActor); err != nil {
			t.Errorf("type %s rejected: %v", typ, err)
		}
	}
}

func TestTrustCertificationStateCase(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certs")
	cases := []struct {
		name string
		days *int
		want string
	}{
		{"null", nil, ""},
		{"yesterday", intp(-1), "expired"},
		{"today", intp(0), "expiring"},
		{"+30", intp(30), "expiring"},
		{"+31", intp(31), "current"},
	}
	for _, c := range cases {
		in := linkCertInput(c.name)
		if c.days != nil {
			in.ValidUntil = strp(pgDay(t, d, *c.days))
		}
		cert := mustCreateCert(t, d, orgID, in)
		got, err := d.GetTrustCertification(ctx, orgID, cert.ID)
		if err != nil || got.State != c.want {
			t.Errorf("%s: state = %q (%v), want %q", c.name, got.State, err, c.want)
		}
	}
}

func TestTrustCertificationUpdateChangelog(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certu")
	until := pgDay(t, d, 100)
	c := mustCreateCert(t, d, orgID, TrustCertificationInput{
		Label: "Old", LinkType: "certificate", Issuer: "BSI", URL: "https://e.com/a", ValidUntil: &until})

	newUntil := pgDay(t, d, 200)
	upd := TrustCertificationInput{Label: "New", LinkType: "audit_report", Issuer: "BSI",
		URL: "https://e.com/b", ValidUntil: &newUntil}
	got, err := d.UpdateTrustCertification(ctx, orgID, c.ID, upd, certActor)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Label != "New" || got.LinkType != "audit_report" || got.URL != "https://e.com/b" || *got.ValidUntil != newUntil {
		t.Errorf("updated = %+v", got)
	}
	want := []string{
		"create||<nil>|Old",
		"update|label|Old|New",
		"update|link_type|certificate|audit_report",
		"update|valid_until|" + until + "|" + newUntil,
		"update|url|https://e.com/a|https://e.com/b",
	}
	if cl := certChangelog(t, d, orgID, c.ID); strings.Join(cl, "\n") != strings.Join(want, "\n") {
		t.Errorf("changelog =\n%s\nwant\n%s", strings.Join(cl, "\n"), strings.Join(want, "\n"))
	}

	// Clearing a date (nil) is a change; an identical update writes nothing.
	upd.ValidUntil = nil
	if got, err = d.UpdateTrustCertification(ctx, orgID, c.ID, upd, certActor); err != nil || got.ValidUntil != nil {
		t.Fatalf("clear date: %+v, %v", got, err)
	}
	n := len(certChangelog(t, d, orgID, c.ID))
	if _, err = d.UpdateTrustCertification(ctx, orgID, c.ID, upd, certActor); err != nil {
		t.Fatal(err)
	}
	if m := len(certChangelog(t, d, orgID, c.ID)); m != n {
		t.Errorf("no-op update wrote %d changelog rows", m-n)
	}

	// Validation failures change nothing.
	bad := upd
	bad.LinkType = "contract"
	if _, err = d.UpdateTrustCertification(ctx, orgID, c.ID, bad, certActor); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("contract update err = %v", err)
	}
	// The source is fixed: a link certification keeps a url, and cannot gain a file.
	noURL := upd
	noURL.URL = ""
	if _, err = d.UpdateTrustCertification(ctx, orgID, c.ID, noURL, certActor); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("link without url err = %v", err)
	}
	withFile := upd
	withFile.File = certFile("x")
	if _, err = d.UpdateTrustCertification(ctx, orgID, c.ID, withFile, certActor); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("update with file err = %v", err)
	}
	// A file certification takes no url.
	fc := mustCreateCert(t, d, orgID, TrustCertificationInput{Label: "F", LinkType: "other", File: certFile("f")})
	if _, err = d.UpdateTrustCertification(ctx, orgID, fc.ID, TrustCertificationInput{Label: "F", LinkType: "other", URL: "https://e.com"}, certActor); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("file certification with url err = %v", err)
	}
	if _, err = d.UpdateTrustCertification(ctx, orgID, fc.ID, TrustCertificationInput{Label: "F2", LinkType: "other"}, certActor); err != nil {
		t.Errorf("file certification metadata update: %v", err)
	}
}

func TestTrustCertificationVerify(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "certverify")
	c, err := d.CreateTrustCertification(ctx, orgID, linkCertInput("V"), email)
	if err != nil || c.VerifiedAt != nil || c.VerifiedBy != "" {
		t.Fatalf("create = %+v, %v", c, err)
	}
	got, err := d.VerifyTrustCertification(ctx, orgID, c.ID, email)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if today := pgDay(t, d, 0); got.VerifiedAt == nil || *got.VerifiedAt != today || got.VerifiedBy != email {
		t.Errorf("verified = %v by %q, want %s by %s", got.VerifiedAt, got.VerifiedBy, today, email)
	}
	cl := certChangelog(t, d, orgID, c.ID)
	if len(cl) != 2 || !strings.HasPrefix(cl[1], "update|verified_at|<nil>|") {
		t.Errorf("changelog = %v", cl)
	}
	if _, err := d.VerifyTrustCertification(ctx, orgID+9999, c.ID, email); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("other org verify err = %v", err)
	}
}

func TestTrustCertificationReplaceFile(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certr")
	c := mustCreateCert(t, d, orgID, TrustCertificationInput{Label: "R", LinkType: "other", File: certFile("old")})

	nf := certFile("new")
	nf.ContentType, nf.Size = "image/png", 99
	got, oldKey, err := d.ReplaceTrustCertificationFile(ctx, orgID, c.ID, *nf, certActor)
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if oldKey != "org/trust/certifications/old.pdf" || got.FileKey != nf.Key || got.FileContentType != "image/png" || got.FileSize != 99 {
		t.Errorf("replaced = %+v, old key %q", got, oldKey)
	}
	cl := certChangelog(t, d, orgID, c.ID)
	if len(cl) != 3 || cl[1] != "update|file_name|old.pdf|new.pdf" || !strings.HasPrefix(cl[2], "update|file_sha256|") {
		t.Errorf("changelog = %v", cl)
	}
	// A link certification has no file to replace; an invalid type is refused.
	lc := mustCreateCert(t, d, orgID, linkCertInput("L"))
	if _, _, err := d.ReplaceTrustCertificationFile(ctx, orgID, lc.ID, *nf, certActor); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("replace on link err = %v", err)
	}
	bad := *nf
	bad.ContentType = "text/html"
	if _, _, err := d.ReplaceTrustCertificationFile(ctx, orgID, c.ID, bad, certActor); !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("replace bad type err = %v", err)
	}
	if _, _, err := d.ReplaceTrustCertificationFile(ctx, orgID+9999, c.ID, *nf, certActor); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("other org replace err = %v", err)
	}
}

func TestTrustCertificationPublishUnpublishDelete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certp")
	otherOrg := trustTestOrg(t, d, "certp2")
	c := mustCreateCert(t, d, orgID, TrustCertificationInput{Label: "P", LinkType: "certificate", File: certFile("p")})

	// Unpublished certifications exist and list with no item.
	list, err := d.ListTrustCertifications(ctx, orgID)
	if err != nil || len(list) != 1 || list[0].Item != nil {
		t.Fatalf("list = %+v, %v", list, err)
	}

	// Another org's certification is not found, and nothing gets published.
	if _, err := d.PublishTrustCertification(ctx, otherOrg, c.ID, TrustAccessPublic, certActor); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("cross-org publish err = %v", err)
	}
	if items, _ := d.ListPublishedTrustCenterItems(ctx, otherOrg); len(items) != 0 {
		t.Errorf("cross-org publish left %d items", len(items))
	}
	if _, err := d.GetTrustCertification(ctx, otherOrg, c.ID); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("cross-org get err = %v", err)
	}
	if l, _ := d.ListTrustCertifications(ctx, otherOrg); len(l) != 0 {
		t.Errorf("cross-org list = %d rows", len(l))
	}

	it, err := d.PublishTrustCertification(ctx, orgID, c.ID, TrustAccessPublic, certActor)
	if err != nil || it.Kind != TrustKindCertification || it.Reference != fmt.Sprint(c.ID) || it.PublicID == "" {
		t.Fatalf("publish = %+v, %v", it, err)
	}
	if _, err := d.PublishTrustCertification(ctx, orgID, c.ID, TrustAccessPublic, certActor); !errors.Is(err, ErrTrustItemAlreadyPublished) {
		t.Errorf("republish err = %v", err)
	}
	if _, err := d.PublishTrustCertification(ctx, orgID, 999999999, TrustAccessPublic, certActor); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("unknown id err = %v", err)
	}
	got, _ := d.GetTrustCertification(ctx, orgID, c.ID)
	if got.Item == nil || got.Item.PublicID != it.PublicID || got.Item.AccessLevel != TrustAccessPublic || got.Item.SortOrder != 1 {
		t.Errorf("joined item = %+v", got.Item)
	}

	// Unpublish keeps the certification.
	if err := d.UnpublishTrustCertification(ctx, orgID, c.ID, certActor); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	if err := d.UnpublishTrustCertification(ctx, orgID, c.ID, certActor); !errors.Is(err, ErrTrustItemNotFound) {
		t.Errorf("second unpublish err = %v", err)
	}
	if got, err := d.GetTrustCertification(ctx, orgID, c.ID); err != nil || got.Item != nil {
		t.Errorf("after unpublish = %+v, %v", got, err)
	}

	// Delete removes the item row in the same transaction and returns the key.
	if _, err := d.PublishTrustCertification(ctx, orgID, c.ID, TrustAccessGated, certActor); err != nil {
		t.Fatal(err)
	}
	if _, err := d.DeleteTrustCertification(ctx, otherOrg, c.ID, certActor); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("cross-org delete err = %v", err)
	}
	key, err := d.DeleteTrustCertification(ctx, orgID, c.ID, certActor)
	if err != nil || key != "org/trust/certifications/p.pdf" {
		t.Fatalf("delete = %q, %v", key, err)
	}
	if _, err := d.GetTrustCertification(ctx, orgID, c.ID); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("get after delete err = %v", err)
	}
	if items, _ := d.ListPublishedTrustCenterItems(ctx, orgID); len(items) != 0 {
		t.Errorf("delete left %d item rows", len(items))
	}
	var itemDeletes int
	_ = d.pool.QueryRow(ctx, `SELECT count(*) FROM entity_changelog WHERE organization_id = $1
		AND entity_type = 'trust_center_item' AND action = 'delete' AND old_value = $2`,
		orgID, "certification:"+fmt.Sprint(c.ID)).Scan(&itemDeletes)
	if itemDeletes != 2 { // the explicit unpublish and the delete
		t.Errorf("item delete changelog rows = %d, want 2", itemDeletes)
	}
	cl := certChangelog(t, d, orgID, c.ID)
	if last := cl[len(cl)-1]; last != "delete||P|<nil>" {
		t.Errorf("last changelog = %q", last)
	}
	// A link certification returns an empty key.
	lc := mustCreateCert(t, d, orgID, linkCertInput("L"))
	if key, err := d.DeleteTrustCertification(ctx, orgID, lc.ID, certActor); err != nil || key != "" {
		t.Errorf("link delete = %q, %v", key, err)
	}
}

func TestTrustCertificationServable(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certsv")
	otherOrg := trustTestOrg(t, d, "certsv2")

	mk := func(label string, days *int, publish bool) (*TrustCertification, *TrustCenterItem) {
		in := linkCertInput(label)
		if days != nil {
			in.ValidUntil = strp(pgDay(t, d, *days))
		}
		c := mustCreateCert(t, d, orgID, in)
		var it *TrustCenterItem
		if publish {
			var err error
			if it, err = d.PublishTrustCertification(ctx, orgID, c.ID, TrustAccessPublic, certActor); err != nil {
				t.Fatal(err)
			}
		}
		return c, it
	}
	_, _ = mk("unpublished", intp(100), false)
	_, _ = mk("yesterday", intp(-1), true)
	_, _ = mk("today", intp(0), true)
	_, _ = mk("future", intp(90), true)
	_, _ = mk("null", nil, true)
	hiddenC, hiddenIt := mk("hidden", intp(90), true)
	if err := d.HideTrustCenterItem(ctx, orgID, hiddenIt.ID, TrustHiddenRetired, "system"); err != nil {
		t.Fatal(err)
	}
	// Another org's certification never shows up.
	oc, err := d.CreateTrustCertification(ctx, otherOrg, linkCertInput("other"), certActor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.PublishTrustCertification(ctx, otherOrg, oc.ID, TrustAccessPublic, certActor); err != nil {
		t.Fatal(err)
	}

	list, err := d.ListServableTrustCertifications(ctx, orgID)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, c := range list {
		labels = append(labels, c.Label)
		if c.Item == nil || c.Item.PublicID == "" {
			t.Errorf("%s: servable without item: %+v", c.Label, c)
		}
	}
	if got, want := strings.Join(labels, ","), "today,future,null"; got != want {
		t.Errorf("servable = %s, want %s (display order)", got, want)
	}

	// Single read by public id: same rules.
	for _, c := range list {
		if got, err := d.GetServableTrustCertification(ctx, orgID, c.Item.PublicID); err != nil || got.Label != c.Label || got.URL == "" {
			t.Errorf("get %s = %+v, %v", c.Label, got, err)
		}
		if _, err := d.GetServableTrustCertification(ctx, otherOrg, c.Item.PublicID); !errors.Is(err, ErrTrustCertificationNotFound) {
			t.Errorf("%s via other org err = %v", c.Label, err)
		}
	}
	var yesterdayPub, hiddenPub string
	_ = d.pool.QueryRow(ctx, `SELECT public_id::text FROM trust_center_items i JOIN trust_center_certifications c
		ON i.reference = c.id::text WHERE i.organization_id = $1 AND i.kind = 'certification' AND c.label = 'yesterday'`, orgID).Scan(&yesterdayPub)
	_ = d.pool.QueryRow(ctx, `SELECT public_id::text FROM trust_center_items WHERE organization_id = $1 AND reference = $2`,
		orgID, fmt.Sprint(hiddenC.ID)).Scan(&hiddenPub)
	for name, id := range map[string]string{"expired": yesterdayPub, "hidden": hiddenPub, "not a uuid": "nope", "unknown": "00000000-0000-0000-0000-000000000000"} {
		if _, err := d.GetServableTrustCertification(ctx, orgID, id); !errors.Is(err, ErrTrustCertificationNotFound) {
			t.Errorf("%s: err = %v, want not found", name, err)
		}
	}
	// A document item with the same public-id shape is not a certification.
	doc := trustPublish(t, d, orgID, TrustKindDocument, "d-1", TrustAccessPublic)
	if _, err := d.GetServableTrustCertification(ctx, orgID, doc.PublicID); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("document item err = %v", err)
	}
	// Renewal brings it back by itself: no write other than valid_until.
	up := linkCertInput("yesterday")
	up.ValidUntil = strp(pgDay(t, d, 365))
	var yid int64
	_ = d.pool.QueryRow(ctx, `SELECT id FROM trust_center_certifications WHERE organization_id = $1 AND label = 'yesterday'`, orgID).Scan(&yid)
	if _, err := d.UpdateTrustCertification(ctx, orgID, yid, up, certActor); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetServableTrustCertification(ctx, orgID, yesterdayPub); err != nil {
		t.Errorf("renewed certification not served: %v", err)
	}
}

func TestOverdueSummaryTrustCertifications(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "certoverdue")
	other := trustTestOrg(t, d, "certoverdue2")

	mk := func(org int, label string, days *int) *TrustCertification {
		in := linkCertInput(label)
		in.Issuer = "Issuer " + label
		if days != nil {
			in.ValidUntil = strp(pgDay(t, d, *days))
		}
		c, err := d.CreateTrustCertification(ctx, org, in, certActor)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	exp := mk(orgID, "expired", intp(-3))
	soon := mk(orgID, "soon", intp(30))
	mk(orgID, "later", intp(31))
	mk(orgID, "nodate", nil)
	mk(other, "other-org", intp(-1))
	// An unpublished certification counts too; publishing changes nothing.
	pub := mk(orgID, "published", intp(5))
	if _, err := d.PublishTrustCertification(ctx, orgID, pub.ID, TrustAccessPublic, certActor); err != nil {
		t.Fatal(err)
	}

	for _, v := range []TaskViewer{{CanSeeAll: true}, {}} {
		s, err := d.GetOverdueSummary(ctx, orgID, v)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.TrustCertifications) != 3 {
			t.Fatalf("bucket = %+v", s.TrustCertifications)
		}
		e, p, so := s.TrustCertifications[0], s.TrustCertifications[1], s.TrustCertifications[2]
		if e.Title != "expired" || e.State != "expired" || e.DaysLate != 3 || e.ParentTitle != "Issuer expired" ||
			e.LinkID != exp.ID || e.EntityID != fmt.Sprintf("CERT-%d", exp.ID) || e.EntityType != "trust_certification" ||
			e.NextReview == nil || e.Owner != "" {
			t.Errorf("expired item = %+v", e)
		}
		if p.Title != "published" || p.State != "expiring" || p.DaysLate != -5 {
			t.Errorf("published item = %+v", p)
		}
		if so.LinkID != soon.ID || so.State != "expiring" || so.DaysLate != -30 {
			t.Errorf("soon item = %+v", so)
		}
		if nd := e.NextReview.UTC().Format("2006-01-02"); nd != pgDay(t, d, -3) && nd != pgDay(t, d, -4) {
			t.Errorf("next_review %s not near the expiry date", nd)
		}
		if s.TotalCount != 3 {
			t.Errorf("TotalCount = %d, want 3", s.TotalCount)
		}
	}
}

func TestCreateOverdueReviewTasksTrustCertifications(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	fallback := "certtask-fallback@notifications-roundtrip.test"
	t.Cleanup(func() { _, _ = d.pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, fallback) })
	orgID, _ := newTestOrgUser(t, d, "certtask")
	if _, err := d.pool.Exec(ctx, `INSERT INTO users (email, name) VALUES ($1, $1)`, fallback); err != nil {
		t.Fatal(err)
	}
	c, err := d.CreateTrustCertification(ctx, orgID, TrustCertificationInput{
		Label: "SOC 2 report", LinkType: "audit_report", Issuer: "Auditor", URL: "https://e.com",
		ValidUntil: strp(pgDay(t, d, -2))}, certActor)
	if err != nil {
		t.Fatal(err)
	}

	// No fallback: counted Unassigned, nothing created (this is how the cron skips it).
	res, err := d.CreateOverdueReviewTasks(ctx, orgID, "system", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Unassigned != 1 || len(res.Created) != 0 {
		t.Fatalf("no fallback: %+v", res)
	}

	res, err = d.CreateOverdueReviewTasks(ctx, orgID, "system", fallback)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Created) != 1 || res.Failed != 0 {
		t.Fatalf("with fallback: %+v", res)
	}
	task := res.Created[0]
	if task.Title != "Renewal: SOC 2 report (trust center)" || task.TaskType != "review" || task.Priority != "medium" ||
		task.Assignee != fallback || task.DueDate == nil {
		t.Errorf("task = %+v", task)
	}
	if due := task.DueDate.UTC().Format("2006-01-02"); due != pgDay(t, d, -2) && due != pgDay(t, d, -3) {
		t.Errorf("due %s not near the expiry date", due)
	}

	// Second run: deduplicated.
	res, err = d.CreateOverdueReviewTasks(ctx, orgID, "system", fallback)
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || len(res.Created) != 0 {
		t.Errorf("second run: %+v", res)
	}
	_ = c
}

// A delete that commits while a publish is in flight must not leave an item
// row pointing at a certification that no longer exists.
func TestTrustCertificationPublishRacingDelete(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certrace")
	c := mustCreateCert(t, d, orgID, linkCertInput("Race"))

	// An open transaction deletes the certification and holds the row lock.
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM trust_center_certifications WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := d.PublishTrustCertification(ctx, orgID, c.ID, TrustAccessPublic, certActor)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("publish returned %v before the delete committed: it did not wait for the row lock", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("publish after the delete committed = %v, want ErrTrustCertificationNotFound", err)
	}
	var n int
	_ = d.pool.QueryRow(ctx, `SELECT count(*) FROM trust_center_items WHERE organization_id = $1 AND kind = 'certification'`, orgID).Scan(&n)
	if n != 0 {
		t.Errorf("orphan certification items = %d, want 0", n)
	}
}

// The mirror order: a publish that commits while a delete waits for the lock is
// removed by that delete.
func TestTrustCertificationDeleteAfterRacingPublish(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certrace2")
	c := mustCreateCert(t, d, orgID, linkCertInput("Race2"))

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM trust_center_certifications WHERE id = $1 FOR SHARE`, c.ID); err != nil {
		t.Fatal(err)
	}
	// Deleting needs FOR UPDATE, which waits for our share lock.
	done := make(chan error, 1)
	go func() {
		_, err := d.DeleteTrustCertification(ctx, orgID, c.ID, certActor)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("delete returned %v while a publish held the certification", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := tx.Exec(ctx, `INSERT INTO trust_center_items (organization_id, kind, reference, access_level, published_by)
		VALUES ($1, 'certification', $2, 'public', 'x')`, orgID, fmt.Sprint(c.ID)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int
	_ = d.pool.QueryRow(ctx, `SELECT count(*) FROM trust_center_items WHERE organization_id = $1 AND kind = 'certification'`, orgID).Scan(&n)
	if n != 0 {
		t.Errorf("orphan certification items = %d, want 0", n)
	}
}

// A partial update merges into the row as it is when the lock is taken: a patch
// that read nothing earlier cannot put back what another update just changed.
func TestTrustCertificationPatchMergesFromTheLockedRow(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID := trustTestOrg(t, d, "certpatch")
	until := pgDay(t, d, 100)
	c := mustCreateCert(t, d, orgID, TrustCertificationInput{
		Label: "P", LinkType: "certificate", Issuer: "BSI", URL: "https://e.com/a", ValidUntil: &until})
	if _, err := d.PublishTrustCertification(ctx, orgID, c.ID, TrustAccessPublic, certActor); err != nil {
		t.Fatal(err)
	}

	// Another transaction holds the row and expires the certification.
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT 1 FROM trust_center_certifications WHERE id = $1 FOR UPDATE`, c.ID); err != nil {
		t.Fatal(err)
	}
	type res struct {
		c   *TrustCertification
		err error
	}
	done := make(chan res, 1)
	issuer := "New issuer"
	go func() {
		got, err := d.PatchTrustCertification(ctx, orgID, c.ID, UpdateTrustCertificationPatch{Issuer: &issuer}, certActor)
		done <- res{got, err}
	}()
	select {
	case r := <-done:
		t.Fatalf("patch returned (%v) while the row was locked: it did not wait for the lock", r.err)
	case <-time.After(300 * time.Millisecond):
	}
	yesterday := pgDay(t, d, -1)
	if _, err := tx.Exec(ctx, `UPDATE trust_center_certifications SET valid_until = $2::date WHERE id = $1`, c.ID, yesterday); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.c.Issuer != issuer || r.c.ValidUntil == nil || *r.c.ValidUntil != yesterday || r.c.State != "expired" {
		t.Errorf("patch undid the concurrent change: %+v", r.c)
	}
	cl := certChangelog(t, d, orgID, c.ID)
	if last := cl[len(cl)-1]; last != "update|issuer|BSI|New issuer" {
		t.Errorf("changelog = %v: want only the issuer change recorded", cl)
	}
	// Absent keeps, set replaces, set-nil clears; a url for a file certification is refused.
	nd := pgDay(t, d, 5)
	got, err := d.PatchTrustCertification(ctx, orgID, c.ID, UpdateTrustCertificationPatch{ValidUntil: OptionalTrustDate{Set: true, Value: &nd}}, certActor)
	if err != nil || *got.ValidUntil != nd || got.Issuer != issuer {
		t.Errorf("set date: %+v %v", got, err)
	}
	got, err = d.PatchTrustCertification(ctx, orgID, c.ID, UpdateTrustCertificationPatch{ValidUntil: OptionalTrustDate{Set: true}}, certActor)
	if err != nil || got.ValidUntil != nil {
		t.Errorf("clear date: %+v %v", got, err)
	}
	fc := mustCreateCert(t, d, orgID, TrustCertificationInput{Label: "F", LinkType: "other", File: certFile("pf")})
	u := "https://e.com"
	if _, err := d.PatchTrustCertification(ctx, orgID, fc.ID, UpdateTrustCertificationPatch{URL: &u}, certActor); !errors.Is(err, ErrTrustCertificationSource) || !errors.Is(err, ErrTrustCenterInvalid) {
		t.Errorf("url on file cert err = %v", err)
	}
	if _, err := d.PatchTrustCertification(ctx, orgID+9999, c.ID, UpdateTrustCertificationPatch{Issuer: &issuer}, certActor); !errors.Is(err, ErrTrustCertificationNotFound) {
		t.Errorf("other org err = %v", err)
	}
}
