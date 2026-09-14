package db

import (
	"context"
	"testing"
)

// offsetDays sets contract_expiry to CURRENT_DATE + n, computed by Postgres so
// the test and the query under test agree on what "today" is regardless of the
// Go process's timezone.
func offsetDays(t *testing.T, d *DB, supplierID int64, n int) {
	t.Helper()
	if _, err := d.pool.Exec(context.Background(),
		`UPDATE suppliers SET contract_expiry = CURRENT_DATE + $1::int WHERE id = $2`, n, supplierID,
	); err != nil {
		t.Fatalf("setting contract_expiry: %v", err)
	}
}

func newTestSupplier(t *testing.T, d *DB, orgID int, name string) *Supplier {
	t.Helper()
	s := &Supplier{
		Name:         name,
		SupplierType: "saas",
		Criticality:  "medium",
		Status:       "active",
	}
	if err := d.CreateSupplier(context.Background(), orgID, s); err != nil {
		t.Fatalf("creating supplier: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.pool.Exec(context.Background(), `DELETE FROM suppliers WHERE id = $1`, s.ID)
	})
	return s
}

func TestGetOverdueSummarySurfacesSupplierContracts(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "contract-overdue")

	expired := newTestSupplier(t, d, orgID, "Expired Co")
	offsetDays(t, d, expired.ID, -5)

	expiring := newTestSupplier(t, d, orgID, "Expiring Co")
	offsetDays(t, d, expiring.ID, 10)

	today := newTestSupplier(t, d, orgID, "Today Co")
	offsetDays(t, d, today.ID, 0)

	farFuture := newTestSupplier(t, d, orgID, "Far Future Co")
	offsetDays(t, d, farFuture.ID, 45)

	unset := newTestSupplier(t, d, orgID, "Unset Co")
	_ = unset

	terminated := newTestSupplier(t, d, orgID, "Terminated Co")
	offsetDays(t, d, terminated.ID, -5)
	if _, err := d.pool.Exec(ctx, `UPDATE suppliers SET status = 'terminated' WHERE id = $1`, terminated.ID); err != nil {
		t.Fatalf("setting status: %v", err)
	}

	// NOTE: the plan called for a "NULL status" case exercising the COALESCE in
	// ruling 1.6, but suppliers.status is NOT NULL DEFAULT 'active' with a CHECK
	// constraint in this schema (migrations/20260327000000_initial_schema.sql:779)
	// — the database itself refuses a NULL status, so that case cannot be
	// constructed. The COALESCE in the query stays as defensive code (harmless,
	// and cheap insurance against a future schema relaxation) but there is no
	// live row to exercise it against.

	summary, err := d.GetOverdueSummary(ctx, orgID, TaskViewer{CanSeeAll: true})
	if err != nil {
		t.Fatalf("GetOverdueSummary: %v", err)
	}

	byIdentifier := map[string]OverdueItem{}
	for _, item := range summary.SupplierContracts {
		byIdentifier[item.EntityID] = item
	}

	if item, ok := byIdentifier[expired.Identifier]; !ok {
		t.Errorf("expired supplier not present")
	} else if item.State != "expired" || item.DaysLate != 5 {
		t.Errorf("expired supplier: got state=%q daysLate=%d, want expired/5", item.State, item.DaysLate)
	}

	if item, ok := byIdentifier[expiring.Identifier]; !ok {
		t.Errorf("expiring supplier not present")
	} else if item.State != "expiring" || item.DaysLate != -10 {
		t.Errorf("expiring supplier: got state=%q daysLate=%d, want expiring/-10", item.State, item.DaysLate)
	}

	if item, ok := byIdentifier[today.Identifier]; !ok {
		t.Errorf("expires-today supplier not present")
	} else if item.State != "expiring" || item.DaysLate != 0 {
		t.Errorf("expires-today supplier: got state=%q daysLate=%d, want expiring/0", item.State, item.DaysLate)
	}

	if _, ok := byIdentifier[farFuture.Identifier]; ok {
		t.Errorf("far-future supplier should be absent")
	}
	if _, ok := byIdentifier[unset.Identifier]; ok {
		t.Errorf("unset-expiry supplier should be absent")
	}
	if _, ok := byIdentifier[terminated.Identifier]; ok {
		t.Errorf("terminated supplier should be absent")
	}

	if summary.TotalCount < len(summary.SupplierContracts) {
		t.Errorf("TotalCount %d does not appear to include SupplierContracts (%d)", summary.TotalCount, len(summary.SupplierContracts))
	}
}

func TestSupplierContractsAtMarksMatchesOnlyDiscreteMarks(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "contract-marks")

	mark30 := newTestSupplier(t, d, orgID, "Mark 30")
	offsetDays(t, d, mark30.ID, 30)
	mark7 := newTestSupplier(t, d, orgID, "Mark 7")
	offsetDays(t, d, mark7.ID, 7)
	mark0 := newTestSupplier(t, d, orgID, "Mark 0")
	offsetDays(t, d, mark0.ID, 0)

	notMarks := map[string]*Supplier{}
	for _, n := range []int{31, 29, 8, -1} {
		s := newTestSupplier(t, d, orgID, "Not Mark")
		offsetDays(t, d, s.ID, n)
		notMarks[s.Identifier] = s
	}

	due, err := d.SupplierContractsAtMarks(ctx, orgID)
	if err != nil {
		t.Fatalf("SupplierContractsAtMarks: %v", err)
	}

	byIdentifier := map[string]SupplierContractDue{}
	for _, c := range due {
		byIdentifier[c.Identifier] = c
	}

	if c, ok := byIdentifier[mark30.Identifier]; !ok || c.DaysUntil != 30 {
		t.Errorf("mark30: got %+v, ok=%v", c, ok)
	}
	if c, ok := byIdentifier[mark7.Identifier]; !ok || c.DaysUntil != 7 {
		t.Errorf("mark7: got %+v, ok=%v", c, ok)
	}
	if c, ok := byIdentifier[mark0.Identifier]; !ok || c.DaysUntil != 0 {
		t.Errorf("mark0: got %+v, ok=%v", c, ok)
	}
	for identifier := range notMarks {
		if _, ok := byIdentifier[identifier]; ok {
			t.Errorf("supplier %s should not be returned", identifier)
		}
	}
}

func TestNotificationExistsTodayDiscriminatesBySupplier(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "contract-dedup")

	const key30 = "notifications.supplier_contract_30"
	const key7 = "notifications.supplier_contract_7"

	if err := d.CreateNotificationContentByEmail(ctx, orgID, email, NotificationContent{
		Title:    "Contract expires in 30 days: Acme",
		TitleKey: key30,
		Params:   map[string]any{"title": "Acme", "id": "SUPPLIER-1"},
	}); err != nil {
		t.Fatalf("creating notification: %v", err)
	}

	exists, err := d.NotificationExistsToday(ctx, orgID, email, key30, "SUPPLIER-1")
	if err != nil {
		t.Fatalf("NotificationExistsToday: %v", err)
	}
	if !exists {
		t.Errorf("expected true for the same mark, same supplier, same day")
	}

	exists, err = d.NotificationExistsToday(ctx, orgID, email, key30, "SUPPLIER-2")
	if err != nil {
		t.Fatalf("NotificationExistsToday: %v", err)
	}
	if exists {
		t.Errorf("expected false for a different supplier — this is the regression that matters")
	}

	exists, err = d.NotificationExistsToday(ctx, orgID, email, key7, "SUPPLIER-1")
	if err != nil {
		t.Fatalf("NotificationExistsToday: %v", err)
	}
	if exists {
		t.Errorf("expected false for a different wire key")
	}
}
