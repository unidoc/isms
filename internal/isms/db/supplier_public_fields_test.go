package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// #522: suppliers carry four public-facing free-text fields and an internal
// trust_public_changed_at that moves only when the visible projection changes
// (public name falling back to name, purpose, location, data categories). The
// timestamp is computed in the UPDATE SQL of both writers.
//
// Requires a migrated Postgres; skipped otherwise:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/db/...

func supplierChangedAt(t *testing.T, d *DB, id int64) time.Time {
	t.Helper()
	var ts *time.Time
	if err := d.pool.QueryRow(context.Background(),
		`SELECT trust_public_changed_at FROM suppliers WHERE id = $1`, id).Scan(&ts); err != nil {
		t.Fatalf("reading trust_public_changed_at: %v", err)
	}
	if ts == nil {
		t.Fatalf("trust_public_changed_at is NULL for supplier %d", id)
	}
	return *ts
}

type supplierWriter struct {
	name string
	run  func(t *testing.T, d *DB, orgID int, s *Supplier)
}

func supplierWriters() []supplierWriter {
	return []supplierWriter{
		{"UpdateSupplier", func(t *testing.T, d *DB, orgID int, s *Supplier) {
			t.Helper()
			if err := d.UpdateSupplier(context.Background(), orgID, s, nil); err != nil {
				t.Fatalf("UpdateSupplier: %v", err)
			}
		}},
		{"UpdateSupplierTx", func(t *testing.T, d *DB, orgID int, s *Supplier) {
			t.Helper()
			err := d.WithOrgTx(context.Background(), orgID, func(ctx context.Context, tx pgx.Tx) error {
				return UpdateSupplierTx(ctx, tx, orgID, s, d.SupplierReviewCycles(ctx, orgID), nil)
			})
			if err != nil {
				t.Fatalf("UpdateSupplierTx: %v", err)
			}
		}},
	}
}

func TestSupplierPublicFieldsRoundTripAndTrim(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "supplier-public-roundtrip")

	s := &Supplier{Name: "Acme", SupplierType: "cloud", Criticality: "low", Status: "active",
		PublicName: "  Acme GmbH ", PublicPurpose: " Cloud hosting ", ProcessingLocation: " EU (Frankfurt) ", DataCategories: " Name, email "}
	if err := d.CreateSupplier(ctx, orgID, s); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	got, err := d.GetSupplier(ctx, orgID, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicName != "Acme GmbH" || got.PublicPurpose != "Cloud hosting" ||
		got.ProcessingLocation != "EU (Frankfurt)" || got.DataCategories != "Name, email" {
		t.Fatalf("create: stored %q %q %q %q, want trimmed values", got.PublicName, got.PublicPurpose, got.ProcessingLocation, got.DataCategories)
	}
	for _, w := range supplierWriters() {
		t.Run(w.name, func(t *testing.T) {
			cur, _ := d.GetSupplier(ctx, orgID, s.ID)
			cur.PublicName, cur.PublicPurpose = "  X Ltd ", "\tPayments\n"
			w.run(t, d, orgID, cur)
			after, _ := d.GetSupplier(ctx, orgID, s.ID)
			if after.PublicName != "X Ltd" || after.PublicPurpose != "Payments" {
				t.Fatalf("stored %q %q, want trimmed", after.PublicName, after.PublicPurpose)
			}
		})
	}
}

func TestSupplierTrustPublicChangedAt(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, _ := newTestOrgUser(t, d, "supplier-public-changed-at")

	for _, w := range supplierWriters() {
		t.Run(w.name, func(t *testing.T) {
			s := &Supplier{Name: "Base " + w.name, SupplierType: "cloud", Criticality: "low", Status: "active"}
			if err := d.CreateSupplier(ctx, orgID, s); err != nil {
				t.Fatalf("CreateSupplier: %v", err)
			}
			created := supplierChangedAt(t, d, s.ID)
			if time.Since(created) > time.Minute || created.After(time.Now().Add(time.Minute)) {
				t.Fatalf("create: trust_public_changed_at = %v, want about now", created)
			}

			last := created
			// step applies edit to the current row, runs the writer and asserts
			// whether trust_public_changed_at moved. The sleep keeps a same-instant
			// update from passing as "did not move".
			step := func(label string, wantMove bool, edit func(*Supplier)) {
				t.Helper()
				time.Sleep(30 * time.Millisecond)
				cur, err := d.GetSupplier(ctx, orgID, s.ID)
				if err != nil {
					t.Fatal(err)
				}
				edit(cur)
				w.run(t, d, orgID, cur)
				now := supplierChangedAt(t, d, s.ID)
				moved := now.After(last)
				if moved != wantMove {
					t.Fatalf("%s: moved = %v, want %v (before %v, after %v)", label, moved, wantMove, last, now)
				}
				last = now
			}

			one, three := 1, 3
			// Not part of the projection.
			step("status", false, func(x *Supplier) { x.Status = "suspended" })
			step("status back", false, func(x *Supplier) { x.Status = "active" })
			step("data_access", false, func(x *Supplier) { x.DataAccess = !x.DataAccess })
			step("criticality", false, func(x *Supplier) { x.Criticality = "high" })
			step("notes", false, func(x *Supplier) { x.Notes = "n" })
			step("contact", false, func(x *Supplier) { x.Contact = "c" })
			step("cia", false, func(x *Supplier) { x.Confidentiality, x.Integrity, x.Availability = &one, &three, &one })
			step("unchanged write", false, func(x *Supplier) {})
			// name feeds the projection while public_name is empty.
			step("name, public_name empty", true, func(x *Supplier) { x.Name = "Renamed " + w.name })
			// Each public field moves it.
			step("public_name", true, func(x *Supplier) { x.PublicName = "Legal Entity Ltd" })
			step("name, public_name set", false, func(x *Supplier) { x.Name = "Renamed again " + w.name })
			step("public_purpose", true, func(x *Supplier) { x.PublicPurpose = "Cloud hosting" })
			step("processing_location", true, func(x *Supplier) { x.ProcessingLocation = "EU" })
			step("data_categories", true, func(x *Supplier) { x.DataCategories = "Email" })
			step("whitespace-only change", false, func(x *Supplier) { x.DataCategories = "  Email " })
			// Clearing public_name falls back to name: the visible name changes.
			step("public_name cleared", true, func(x *Supplier) { x.PublicName = "" })
			step("public_name set equal to name", false, func(x *Supplier) { x.PublicName = x.Name })
		})
	}
}

// A supplier review, and an UpdateSupplier with an otherwise unchanged struct
// (the manager cron path), keep the public fields and leave the timestamp alone.
func TestSupplierReviewKeepsPublicFields(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	orgID, email := newTestOrgUser(t, d, "supplier-public-review")

	s := &Supplier{Name: "Acme", SupplierType: "cloud", Criticality: "low", Status: "active",
		PublicName: "Acme GmbH", PublicPurpose: "Hosting", ProcessingLocation: "EU", DataCategories: "Email"}
	if err := d.CreateSupplier(ctx, orgID, s); err != nil {
		t.Fatal(err)
	}
	before := supplierChangedAt(t, d, s.ID)
	time.Sleep(30 * time.Millisecond)

	sr := &SupplierReview{SupplierID: s.ID, Outcome: "unsatisfactory", ReviewedBy: email}
	if err := d.CreateSupplierReview(ctx, orgID, sr); err != nil {
		t.Fatalf("CreateSupplierReview: %v", err)
	}
	got, _ := d.GetSupplier(ctx, orgID, s.ID)
	if got.Status != "under_review" {
		t.Fatalf("precondition: review should move status, got %q", got.Status)
	}
	if got.PublicName != "Acme GmbH" || got.PublicPurpose != "Hosting" || got.ProcessingLocation != "EU" || got.DataCategories != "Email" {
		t.Fatalf("review blanked the public fields: %+v", got)
	}
	if after := supplierChangedAt(t, d, s.ID); !after.Equal(before) {
		t.Fatalf("review moved trust_public_changed_at: %v -> %v", before, after)
	}

	// Cron-style write: read, change nothing public, write.
	time.Sleep(30 * time.Millisecond)
	if err := d.UpdateSupplier(ctx, orgID, got, nil); err != nil {
		t.Fatal(err)
	}
	if after := supplierChangedAt(t, d, s.ID); !after.Equal(before) {
		t.Fatalf("unchanged UpdateSupplier moved trust_public_changed_at: %v -> %v", before, after)
	}
}

func TestSupplierChangeMapCarriesPublicFields(t *testing.T) {
	s := &Supplier{PublicName: "a", PublicPurpose: "b", ProcessingLocation: "c", DataCategories: "d"}
	m := s.ToChangeMap()
	for k, want := range map[string]string{"public_name": "a", "public_purpose": "b", "processing_location": "c", "data_categories": "d"} {
		if m[k] != want {
			t.Errorf("ToChangeMap[%q] = %q, want %q", k, m[k], want)
		}
	}
	if _, ok := m["trust_public_changed_at"]; ok {
		t.Error("trust_public_changed_at must not be in the change map")
	}
}
