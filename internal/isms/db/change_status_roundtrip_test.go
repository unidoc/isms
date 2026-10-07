package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// Requires a migrated Postgres; skipped when ISMS_TEST_DATABASE_URL is unset
// (see notifications_roundtrip_test.go for how to stand one up).
//
// Regression for #197: moving an approved change to in_progress (or reopening
// an implemented one) used to wipe approved_at / approved_by[_user_id] and
// implemented_at. Only proposed and rejected may revoke an approval. The pool
// and tx variants share one code path and must agree.

func newTestChange(t *testing.T, d *DB, orgID int, requestedBy string) *ChangeRequest {
	t.Helper()
	cr := &ChangeRequest{
		Title: "change status", Description: "d", Priority: "medium", Category: "other",
		RiskLevel: "low", RequestedBy: requestedBy, Status: "proposed",
	}
	if err := d.CreateChangeRequest(context.Background(), orgID, cr); err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	return cr
}

// approvedByUserID reads the raw approved_by_user_id column.
func approvedByUserID(t *testing.T, d *DB, id int) *int {
	t.Helper()
	var uid *int
	if err := d.pool.QueryRow(context.Background(),
		`SELECT approved_by_user_id FROM change_requests WHERE id = $1`, id).Scan(&uid); err != nil {
		t.Fatalf("reading approved_by_user_id: %v", err)
	}
	return uid
}

func TestChangeStatusTransitionsKeepAndClearStamps(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, approver := newTestOrgUser(t, d, "change-status-roundtrip")

	variants := map[string]func(orgID, id int, status string) error{
		"pool": func(orgID, id int, status string) error {
			return d.UpdateChangeRequestStatus(ctx, orgID, id, status, approver)
		},
		"tx": func(orgID, id int, status string) error {
			return d.WithOrgTx(ctx, orgID, func(ctx context.Context, tx pgx.Tx) error {
				return UpdateChangeRequestStatusTx(ctx, tx, orgID, id, status, approver)
			})
		},
	}

	for name, move := range variants {
		t.Run(name, func(t *testing.T) {
			cr := newTestChange(t, d, orgID, approver)
			get := func() *ChangeRequest {
				t.Helper()
				got, err := d.GetChangeRequest(ctx, orgID, cr.ID)
				if err != nil {
					t.Fatalf("GetChangeRequest: %v", err)
				}
				return got
			}
			step := func(status string) *ChangeRequest {
				t.Helper()
				if err := move(orgID, cr.ID, status); err != nil {
					t.Fatalf("move to %s: %v", status, err)
				}
				return get()
			}

			approved := step("approved")
			if approved.ApprovedBy != approver || approved.ApprovedAt == nil {
				t.Fatalf("after approve: approved_by=%q approved_at=%v, want stamp", approved.ApprovedBy, approved.ApprovedAt)
			}
			if approvedByUserID(t, d, cr.ID) == nil {
				t.Fatal("after approve: approved_by_user_id is NULL, want the approver's id")
			}

			inProgress := step("in_progress")
			if inProgress.ApprovedBy != approver || inProgress.ApprovedAt == nil || !inProgress.ApprovedAt.Time.Equal(approved.ApprovedAt.Time) {
				t.Errorf("in_progress lost the approval: by=%q at=%v, want %q at %v", inProgress.ApprovedBy, inProgress.ApprovedAt, approver, approved.ApprovedAt)
			}
			if approvedByUserID(t, d, cr.ID) == nil {
				t.Error("in_progress cleared approved_by_user_id")
			}

			implemented := step("implemented")
			if implemented.ImplementedAt == nil {
				t.Fatal("implemented: implemented_at is nil, want set")
			}
			if implemented.ApprovedAt == nil {
				t.Error("implemented lost the approval")
			}

			reopened := step("in_progress")
			if reopened.ImplementedAt == nil || !reopened.ImplementedAt.Time.Equal(implemented.ImplementedAt.Time) {
				t.Errorf("reopen changed implemented_at: got %v, want %v", reopened.ImplementedAt, implemented.ImplementedAt)
			}
			if reopened.ApprovedBy != approver || reopened.ApprovedAt == nil || !reopened.ApprovedAt.Time.Equal(approved.ApprovedAt.Time) {
				t.Errorf("reopen lost the approval: by=%q at=%v", reopened.ApprovedBy, reopened.ApprovedAt)
			}

			rejected := step("rejected")
			if rejected.ApprovedBy != "" || rejected.ApprovedAt != nil || rejected.ImplementedAt != nil {
				t.Errorf("rejected left stamps: by=%q at=%v implemented=%v", rejected.ApprovedBy, rejected.ApprovedAt, rejected.ImplementedAt)
			}
			if approvedByUserID(t, d, cr.ID) != nil {
				t.Error("rejected left approved_by_user_id set")
			}

			// A second change: sending an approved change back to proposed revokes it.
			other := newTestChange(t, d, orgID, approver)
			if err := move(orgID, other.ID, "approved"); err != nil {
				t.Fatalf("approve second change: %v", err)
			}
			if err := move(orgID, other.ID, "proposed"); err != nil {
				t.Fatalf("propose second change: %v", err)
			}
			back, err := d.GetChangeRequest(ctx, orgID, other.ID)
			if err != nil {
				t.Fatalf("GetChangeRequest: %v", err)
			}
			if back.ApprovedBy != "" || back.ApprovedAt != nil || approvedByUserID(t, d, other.ID) != nil {
				t.Errorf("proposed left an approval: by=%q at=%v", back.ApprovedBy, back.ApprovedAt)
			}
		})
	}
}
