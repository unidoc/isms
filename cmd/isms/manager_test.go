package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"isms.sh/internal/isms/db"
)

// A failed task-creation step must be reported without costing the org its
// contract-expiry notifications. The failure is injected through a private
// search_path schema holding a shadow document_versions view, so the shared
// public schema is untouched.
func TestManageOrgNotifiesWhenTaskCreationFails(t *testing.T) {
	base := os.Getenv("ISMS_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("ISMS_TEST_DATABASE_URL not set — skipping database round-trip test")
	}
	ctx := context.Background()
	admin, err := db.New(ctx, base)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(admin.Close)

	suffix := time.Now().UnixNano()
	slug := fmt.Sprintf("manager-test-%d", suffix)
	email := slug + "@manager-test.test"
	var orgID, userID int
	if err := admin.Pool().QueryRow(ctx,
		`INSERT INTO organizations (name, slug, repo_path) VALUES ($1, $1, '/tmp/none') RETURNING id`, slug).Scan(&orgID); err != nil {
		t.Fatalf("creating org: %v", err)
	}
	if err := admin.Pool().QueryRow(ctx,
		`INSERT INTO users (email, name) VALUES ($1, $1) RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Pool().Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID)
		_, _ = admin.Pool().Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	// An owned supplier whose contract expires in exactly 30 days: one of the
	// marks NotifySupplierContractExpiry fires on.
	sup := &db.Supplier{Name: "Manager Co", SupplierType: "saas", Criticality: "medium", Status: "active"}
	if err := admin.CreateSupplier(ctx, orgID, sup); err != nil {
		t.Fatalf("creating supplier: %v", err)
	}
	if _, err := admin.Pool().Exec(ctx,
		`UPDATE suppliers SET contract_expiry = CURRENT_DATE + 30, owner_id = $1 WHERE id = $2`, userID, sup.ID); err != nil {
		t.Fatalf("seeding contract: %v", err)
	}

	// document_versions without review_cycle_months: CreateOverdueReviewTasks
	// fails after the overdue summary succeeded.
	schema := fmt.Sprintf("manager_shadow_%d", suffix)
	for _, stmt := range []string{
		`CREATE SCHEMA ` + schema,
		`CREATE VIEW ` + schema + `.document_versions AS SELECT s.organization_id, s.document_id, s.version, s.created_by, s.created_at, s.owner FROM public.document_versions s`,
	} {
		if _, err := admin.Pool().Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	t.Cleanup(func() { _, _ = admin.Pool().Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) })

	sep := "?"
	if u, err := url.Parse(base); err == nil && u.RawQuery != "" {
		sep = "&"
	}
	shadow, err := db.New(ctx, base+sep+"options="+strings.ReplaceAll(url.QueryEscape("-c search_path="+schema+",public"), "+", "%20"))
	if err != nil {
		t.Fatalf("connecting shadow pool: %v", err)
	}
	t.Cleanup(shadow.Close)

	_, _, notified, _, _, err := manageOrg(ctx, shadow, orgID, true)
	if err == nil {
		t.Error("want the task-creation error to be returned")
	}
	if notified != 1 {
		t.Errorf("contract-expiry notifications must still run: notified = %d, want 1", notified)
	}
}
