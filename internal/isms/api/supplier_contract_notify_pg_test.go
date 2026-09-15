package api

import (
	"context"
	"testing"

	"isms.sh/internal/isms/db"
)

// contractTestUser creates (or reuses) a user and adds it to orgID with role.
// UpsertUser is exported and idempotent by lower(email), unlike
// db.newTestOrgUser's raw INSERT (unreachable from this package because it
// closes over the unexported pool) — this is the minimal exported-API
// equivalent for the api package.
func contractTestUser(t *testing.T, s *Server, orgID int, email, role string) *db.User {
	t.Helper()
	ctx := context.Background()
	u := &db.User{Email: email, Name: email, Active: true}
	if err := s.db.UpsertUser(ctx, u); err != nil {
		t.Fatalf("creating user %s: %v", email, err)
	}
	if err := s.db.AddOrgMember(ctx, orgID, u.ID, role); err != nil {
		t.Fatalf("adding %s to org %d as %s: %v", email, orgID, role, err)
	}
	return u
}

// offsetSupplierExpiry sets contract_expiry to CURRENT_DATE + n, computed by
// Postgres so the test and NotifySupplierContractExpiry's underlying query
// agree on what "today" is regardless of the Go process's timezone. Setting it
// from Go (time.Now().AddDate(...)) truncates to DATE in the session's
// timezone and can land the value on the wrong day (plan #44, Task 12).
//
// db.newTestOrgUser's sibling helper reaches for the unexported d.pool
// directly, which is not reachable from this package; d.Pool() is the existing
// exported escape hatch (already used by cmd/isms for migrations) so no new
// helper needs to be added to the db package for this.
func offsetSupplierExpiry(t *testing.T, s *Server, supplierID int64, n int) {
	t.Helper()
	if _, err := s.db.Pool().Exec(context.Background(),
		`UPDATE suppliers SET contract_expiry = CURRENT_DATE + $1::int WHERE id = $2`, n, supplierID,
	); err != nil {
		t.Fatalf("setting contract_expiry: %v", err)
	}
}

// notificationsFor returns every notification recorded for recipientEmail in
// orgID, newest first, resolving the email to a user id via ListOrgUsers (the
// same exported path the job itself has no need to bypass).
func notificationsFor(t *testing.T, s *Server, orgID int, email string) []db.Notification {
	t.Helper()
	ctx := context.Background()
	members, err := s.db.ListOrgUsers(ctx, orgID)
	if err != nil {
		t.Fatalf("listing org users: %v", err)
	}
	var userID int
	found := false
	for _, m := range members {
		if m.Email == email {
			userID = m.ID
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no member with email %s in org %d", email, orgID)
	}
	ns, err := s.db.ListNotifications(ctx, orgID, userID, false, 0)
	if err != nil {
		t.Fatalf("listing notifications for %s: %v", email, err)
	}
	return ns
}

func newContractSupplier(t *testing.T, s *Server, orgID int, name, ownerEmail string) *db.Supplier {
	t.Helper()
	sup := &db.Supplier{
		Name:         name,
		SupplierType: db.SupplierTypes[0],
		Criticality:  db.CriticalityLevels[0],
		Status:       db.SupplierStatuses[0],
		Owner:        ownerEmail,
	}
	if err := s.db.CreateSupplier(context.Background(), orgID, sup); err != nil {
		t.Fatalf("creating supplier %s: %v", name, err)
	}
	return sup
}

// TestContractExpiryOwnedSupplierNotifiesOwnerOnly (case 1): a supplier with an
// owner at the +30 mark notifies exactly the owner, not the admin/manager
// fallback.
func TestContractExpiryOwnedSupplierNotifiesOwnerOnly(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "contract-owned")

	owner := contractTestUser(t, s, orgID, "owner@contract-owned.test", "contributor")
	admin := contractTestUser(t, s, orgID, "admin@contract-owned.test", "admin")

	sup := newContractSupplier(t, s, orgID, "Owned Co", owner.Email)
	offsetSupplierExpiry(t, s, sup.ID, 30)

	sent, err := NotifySupplierContractExpiry(ctx, s.db, orgID)
	if err != nil {
		t.Fatalf("NotifySupplierContractExpiry: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}

	ownerNotes := notificationsFor(t, s, orgID, owner.Email)
	if len(ownerNotes) != 1 {
		t.Fatalf("owner has %d notifications, want 1", len(ownerNotes))
	}
	if ownerNotes[0].TitleKey != NotifyKeySupplierContract30 {
		t.Errorf("title_key = %q, want %q", ownerNotes[0].TitleKey, NotifyKeySupplierContract30)
	}
	if got, want := ownerNotes[0].Params, map[string]any{"title": "Owned Co", "id": sup.Identifier}; got["title"] != want["title"] || got["id"] != want["id"] {
		t.Errorf("params = %v, want %v", got, want)
	}

	adminNotes := notificationsFor(t, s, orgID, admin.Email)
	if len(adminNotes) != 0 {
		t.Errorf("admin has %d notifications, want 0 — an owned supplier must not also notify the fallback", len(adminNotes))
	}
}

// TestContractExpiryUnownedSupplierNotifiesAdminsAndManagers (case 2): a
// supplier with no owner at the +7 mark notifies every admin/manager, and does
// not notify a plain member.
func TestContractExpiryUnownedSupplierNotifiesAdminsAndManagers(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "contract-unowned")

	admin := contractTestUser(t, s, orgID, "admin@contract-unowned.test", "admin")
	manager := contractTestUser(t, s, orgID, "manager@contract-unowned.test", "manager")
	reader := contractTestUser(t, s, orgID, "reader@contract-unowned.test", "reader")
	contributor := contractTestUser(t, s, orgID, "contributor@contract-unowned.test", "contributor")

	sup := newContractSupplier(t, s, orgID, "Unowned Co", "")
	offsetSupplierExpiry(t, s, sup.ID, 7)

	sent, err := NotifySupplierContractExpiry(ctx, s.db, orgID)
	if err != nil {
		t.Fatalf("NotifySupplierContractExpiry: %v", err)
	}
	if sent != 2 {
		t.Fatalf("sent = %d, want 2 (admin + manager)", sent)
	}

	for _, u := range []*db.User{admin, manager} {
		ns := notificationsFor(t, s, orgID, u.Email)
		if len(ns) != 1 {
			t.Fatalf("%s has %d notifications, want 1", u.Email, len(ns))
		}
		if ns[0].TitleKey != NotifyKeySupplierContract7 {
			t.Errorf("%s title_key = %q, want %q", u.Email, ns[0].TitleKey, NotifyKeySupplierContract7)
		}
	}
	for _, u := range []*db.User{reader, contributor} {
		ns := notificationsFor(t, s, orgID, u.Email)
		if len(ns) != 0 {
			t.Errorf("%s (role != admin/manager) has %d notifications, want 0", u.Email, len(ns))
		}
	}
}

// TestContractExpirySecondRunSameDayCreatesNothing (case 3): the hourly-cron
// dedup — running the job twice on the same day must not double-notify.
func TestContractExpirySecondRunSameDayCreatesNothing(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "contract-dedup")

	owner := contractTestUser(t, s, orgID, "owner@contract-dedup.test", "contributor")
	sup := newContractSupplier(t, s, orgID, "Dedup Co", owner.Email)
	offsetSupplierExpiry(t, s, sup.ID, 30)

	first, err := NotifySupplierContractExpiry(ctx, s.db, orgID)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first != 1 {
		t.Fatalf("first run sent = %d, want 1", first)
	}

	second, err := NotifySupplierContractExpiry(ctx, s.db, orgID)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second != 0 {
		t.Fatalf("second run sent = %d, want 0 — same-day rerun must not double-notify", second)
	}

	ns := notificationsFor(t, s, orgID, owner.Email)
	if len(ns) != 1 {
		t.Fatalf("owner has %d notifications after two runs, want 1", len(ns))
	}
}

// TestContractExpiryTwoSuppliersSameOwnerBothNotify (case 4): the load-bearing
// regression — two suppliers owned by the same person, both at the +30 mark,
// must produce two notifications, not one collapsed by the per-supplier dedup
// key.
func TestContractExpiryTwoSuppliersSameOwnerBothNotify(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "contract-multi")

	owner := contractTestUser(t, s, orgID, "owner@contract-multi.test", "contributor")
	supA := newContractSupplier(t, s, orgID, "Multi Co A", owner.Email)
	supB := newContractSupplier(t, s, orgID, "Multi Co B", owner.Email)
	offsetSupplierExpiry(t, s, supA.ID, 30)
	offsetSupplierExpiry(t, s, supB.ID, 30)

	sent, err := NotifySupplierContractExpiry(ctx, s.db, orgID)
	if err != nil {
		t.Fatalf("NotifySupplierContractExpiry: %v", err)
	}
	if sent != 2 {
		t.Fatalf("sent = %d, want 2 — one supplier owned by one person must not swallow the other", sent)
	}

	ns := notificationsFor(t, s, orgID, owner.Email)
	if len(ns) != 2 {
		t.Fatalf("owner has %d notifications, want 2", len(ns))
	}
	gotIDs := map[string]bool{}
	for _, n := range ns {
		id, _ := n.Params["id"].(string)
		gotIDs[id] = true
	}
	if !gotIDs[supA.Identifier] || !gotIDs[supB.Identifier] {
		t.Errorf("notifications carry identifiers %v, want both %s and %s", gotIDs, supA.Identifier, supB.Identifier)
	}
}

// TestContractExpiryAlreadyExpiredSupplierNotNotified (case 5): a contract that
// expired days ago is carried by the overdue summary, not re-fired here.
func TestContractExpiryAlreadyExpiredSupplierNotNotified(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "contract-expired")

	owner := contractTestUser(t, s, orgID, "owner@contract-expired.test", "contributor")
	sup := newContractSupplier(t, s, orgID, "Expired Co", owner.Email)
	offsetSupplierExpiry(t, s, sup.ID, -5)

	sent, err := NotifySupplierContractExpiry(ctx, s.db, orgID)
	if err != nil {
		t.Fatalf("NotifySupplierContractExpiry: %v", err)
	}
	if sent != 0 {
		t.Fatalf("sent = %d, want 0 — nothing fires after expiry", sent)
	}
	ns := notificationsFor(t, s, orgID, owner.Email)
	if len(ns) != 0 {
		t.Errorf("owner has %d notifications for an already-expired contract, want 0", len(ns))
	}
}

// TestContractExpiryTitleKeysAndParamsPerMark (case 6): each mark carries its
// own title_key, and params are exactly {title, id} — the supplier's name and
// identifier, nothing else.
func TestContractExpiryTitleKeysAndParamsPerMark(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "contract-keys")
	owner := contractTestUser(t, s, orgID, "owner@contract-keys.test", "contributor")

	cases := []struct {
		name    string
		offset  int
		wantKey string
	}{
		{"Keys Co 30", 30, NotifyKeySupplierContract30},
		{"Keys Co 7", 7, NotifyKeySupplierContract7},
		{"Keys Co 0", 0, NotifyKeySupplierContractToday},
	}

	suppliers := make([]*db.Supplier, len(cases))
	for i, c := range cases {
		suppliers[i] = newContractSupplier(t, s, orgID, c.name, owner.Email)
		offsetSupplierExpiry(t, s, suppliers[i].ID, c.offset)
	}

	sent, err := NotifySupplierContractExpiry(ctx, s.db, orgID)
	if err != nil {
		t.Fatalf("NotifySupplierContractExpiry: %v", err)
	}
	if sent != len(cases) {
		t.Fatalf("sent = %d, want %d", sent, len(cases))
	}

	ns := notificationsFor(t, s, orgID, owner.Email)
	if len(ns) != len(cases) {
		t.Fatalf("owner has %d notifications, want %d", len(ns), len(cases))
	}

	byIdentifier := map[string]db.Notification{}
	for _, n := range ns {
		id, _ := n.Params["id"].(string)
		byIdentifier[id] = n
	}

	for i, c := range cases {
		sup := suppliers[i]
		n, ok := byIdentifier[sup.Identifier]
		if !ok {
			t.Errorf("%s: no notification carrying identifier %s", c.name, sup.Identifier)
			continue
		}
		if n.TitleKey != c.wantKey {
			t.Errorf("%s: title_key = %q, want %q", c.name, n.TitleKey, c.wantKey)
		}
		if len(n.Params) != 2 {
			t.Errorf("%s: params = %v, want exactly title and id", c.name, n.Params)
		}
		if n.Params["title"] != c.name {
			t.Errorf("%s: params[title] = %v, want %q", c.name, n.Params["title"], c.name)
		}
		if n.Params["id"] != sup.Identifier {
			t.Errorf("%s: params[id] = %v, want %q", c.name, n.Params["id"], sup.Identifier)
		}
	}
}
