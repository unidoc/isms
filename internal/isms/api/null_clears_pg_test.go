package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise so `go test ./...` stays green
// without a database:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/...
//
// Regression for #381: the update requests used **T for the three states
// "absent, null, value", but encoding/json decodes null into a **T as an outer
// nil, the same as an absent key, so an explicit null never cleared anything
// and the handler answered 200 with nothing changed. Every nullable field of
// the 13 update requests now has to behave the same way:
//
//   - {} changes nothing.
//   - {"<field>": null} clears that field and leaves its siblings alone.
//   - On the four entities that recalculate next_review from a review cycle
//     (risk, system, supplier, legal requirement), null next_review means
//     "recalculate" and leaves a date, not nil.
//   - A real 0 or false is stored as a value, not read as a clear.
//   - An empty string still clears a date, as it did before.

// nullCaseSeedDate is a fixed instant so the seeded values do not depend on
// today's date.
// nullCaseProgramSeq makes program keys unique, since seeds run many times in
// one organization and the key is unique per organization.
var nullCaseProgramSeq int

func nullCaseProgramKey() string {
	nullCaseProgramSeq++
	return fmt.Sprintf("NULLCLR%d", nullCaseProgramSeq)
}

var nullCaseSeedDate = db.NewEpoch(time.Date(2030, 6, 15, 0, 0, 0, 0, time.UTC))

// nullCaseActor is the user the shared ctxFor helper puts on the request.
const nullCaseActor = "admin@id-resolution.test"

// ptrStr renders a nullable column so a snapshot can be compared as strings.
func ptrStr[T any](p *T) string {
	if p == nil {
		return "<nil>"
	}
	if e, ok := any(*p).(db.Epoch); ok {
		return e.Time.UTC().Format("2006-01-02T15:04:05")
	}
	return fmt.Sprint(*p)
}

// nullEntity describes one update endpoint under test.
type nullEntity struct {
	name    string
	handler func(*Server) func(echo.Context) error
	// seed creates the entity with every nullable field set and returns its id.
	seed func(t *testing.T, s *Server, orgID int) int64
	// snapshot reads the entity back and returns every nullable field the
	// update request carries, keyed by its JSON name.
	snapshot func(t *testing.T, s *Server, orgID int, id int64) map[string]string
	// dates lists the keys that hold a date, which also accept "" as a clear.
	dates []string
	// recalcNextReview marks the entities where null next_review recalculates
	// the date instead of clearing it.
	recalcNextReview bool
	// zeros are bodies carrying a real 0 or false, with the value expected back.
	zeros []nullZero
}

type nullZero struct {
	key  string
	body string
	want string
}

func nullCaseUser(t *testing.T, s *Server, orgID int, email, role string) {
	t.Helper()
	contractTestUser(t, s, orgID, email, role)
}

func nullCases() []nullEntity {
	i := func(v int) *int { return &v }
	d := func(days int) *db.Epoch { e := db.NewEpoch(nullCaseSeedDate.AddDate(0, 0, days)); return &e }

	return []nullEntity{
		{
			name:    "asset",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateAsset },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				a := &db.Asset{Name: "asset", AssetType: db.AssetTypes[0], Status: db.AssetStatuses[0],
					Confidentiality: i(1), Integrity: i(2), Availability: i(3), LastReview: d(1), NextReview: d(2)}
				if err := s.db.CreateAsset(context.Background(), orgID, a); err != nil {
					t.Fatalf("CreateAsset: %v", err)
				}
				return a.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				a, err := s.db.GetAsset(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetAsset: %v", err)
				}
				return map[string]string{
					"confidentiality": ptrStr(a.Confidentiality), "integrity": ptrStr(a.Integrity),
					"availability": ptrStr(a.Availability), "last_review": ptrStr(a.LastReview),
					"next_review": ptrStr(a.NextReview),
				}
			},
			dates: []string{"last_review", "next_review"},
			zeros: []nullZero{{"confidentiality", `{"confidentiality":0}`, "0"}},
		},
		{
			name:    "supplier",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateSupplier },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				sup := &db.Supplier{Name: "supplier", SupplierType: "cloud", Criticality: "low", Status: "active",
					ContractExpiry: d(3), Confidentiality: i(1), Integrity: i(2), Availability: i(3),
					LastReview: d(1), NextReview: d(2)}
				if err := s.db.CreateSupplier(context.Background(), orgID, sup); err != nil {
					t.Fatalf("CreateSupplier: %v", err)
				}
				return sup.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetSupplier(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetSupplier: %v", err)
				}
				return map[string]string{
					"contract_expiry": ptrStr(x.ContractExpiry), "confidentiality": ptrStr(x.Confidentiality),
					"integrity": ptrStr(x.Integrity), "availability": ptrStr(x.Availability),
					"last_review": ptrStr(x.LastReview), "next_review": ptrStr(x.NextReview),
				}
			},
			dates:            []string{"contract_expiry", "last_review", "next_review"},
			recalcNextReview: true,
			zeros:            []nullZero{{"integrity", `{"integrity":0}`, "0"}},
		},
		{
			name:    "system",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateSystem },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				sup := &db.Supplier{Name: "system supplier", SupplierType: "cloud", Criticality: "low", Status: "active"}
				if err := s.db.CreateSupplier(context.Background(), orgID, sup); err != nil {
					t.Fatalf("CreateSupplier: %v", err)
				}
				sys := &db.System{Name: "system", Classification: "internal", Criticality: "low", Status: "active",
					SupplierID: &sup.ID, Confidentiality: i(1), Integrity: i(2), Availability: i(3),
					LastReview: d(1), NextReview: d(2)}
				if err := s.db.CreateSystem(context.Background(), orgID, sys); err != nil {
					t.Fatalf("CreateSystem: %v", err)
				}
				return sys.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetSystem(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetSystem: %v", err)
				}
				return map[string]string{
					"supplier_id": ptrStr(x.SupplierID), "confidentiality": ptrStr(x.Confidentiality),
					"integrity": ptrStr(x.Integrity), "availability": ptrStr(x.Availability),
					"last_review": ptrStr(x.LastReview), "next_review": ptrStr(x.NextReview),
				}
			},
			dates:            []string{"last_review", "next_review"},
			recalcNextReview: true,
			zeros:            []nullZero{{"availability", `{"availability":0}`, "0"}},
		},
		{
			name:    "risk",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateRisk },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				r := &db.Risk{Title: "risk", RiskType: db.RiskTypes[0], Origin: db.RiskOrigins[0], Status: db.RiskStatuses[0],
					Treatment:         "mitigate",
					CurrentLikelihood: i(2), CurrentImpact: i(3),
					ConfidentialityImpact: i(1), IntegrityImpact: i(2), AvailabilityImpact: i(3),
					InherentLikelihood: i(4), InherentImpact: i(4),
					InherentConfidentialityImpact: i(1), InherentIntegrityImpact: i(2), InherentAvailabilityImpact: i(3),
					TargetLikelihood: i(1), TargetImpact: i(1),
					TreatmentDueDate: d(4), LastReview: d(1), NextReview: d(2)}
				if err := s.db.CreateRisk(context.Background(), orgID, r); err != nil {
					t.Fatalf("CreateRisk: %v", err)
				}
				return r.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetRisk(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetRisk: %v", err)
				}
				return map[string]string{
					"current_likelihood": ptrStr(x.CurrentLikelihood), "current_impact": ptrStr(x.CurrentImpact),
					"confidentiality_impact": ptrStr(x.ConfidentialityImpact), "integrity_impact": ptrStr(x.IntegrityImpact),
					"availability_impact":             ptrStr(x.AvailabilityImpact),
					"inherent_likelihood":             ptrStr(x.InherentLikelihood),
					"inherent_impact":                 ptrStr(x.InherentImpact),
					"inherent_confidentiality_impact": ptrStr(x.InherentConfidentialityImpact),
					"inherent_integrity_impact":       ptrStr(x.InherentIntegrityImpact),
					"inherent_availability_impact":    ptrStr(x.InherentAvailabilityImpact),
					"target_likelihood":               ptrStr(x.TargetLikelihood), "target_impact": ptrStr(x.TargetImpact),
					"treatment_due_date": ptrStr(x.TreatmentDueDate), "last_review": ptrStr(x.LastReview),
					"next_review": ptrStr(x.NextReview),
				}
			},
			dates:            []string{"treatment_due_date", "last_review", "next_review"},
			recalcNextReview: true,
			zeros: []nullZero{
				{"current_likelihood", `{"current_likelihood":0}`, "0"},
				{"target_impact", `{"target_impact":0}`, "0"},
			},
		},
		{
			name:    "legal requirement",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateLegal },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				l := &db.LegalRequirement{Title: "legal", Jurisdiction: "EU", Category: "privacy", Status: "open",
					LastReview: d(1), NextReview: d(2),
					CurrentLikelihood: i(2), CurrentImpact: i(3), TargetLikelihood: i(1), TargetImpact: i(1)}
				if err := s.db.CreateLegalRequirement(context.Background(), orgID, l); err != nil {
					t.Fatalf("CreateLegalRequirement: %v", err)
				}
				return l.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetLegalRequirement(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetLegalRequirement: %v", err)
				}
				return map[string]string{
					"last_review": ptrStr(x.LastReview), "next_review": ptrStr(x.NextReview),
					"current_likelihood": ptrStr(x.CurrentLikelihood), "current_impact": ptrStr(x.CurrentImpact),
					"target_likelihood": ptrStr(x.TargetLikelihood), "target_impact": ptrStr(x.TargetImpact),
				}
			},
			dates:            []string{"last_review", "next_review"},
			recalcNextReview: true,
			zeros:            []nullZero{{"current_impact", `{"current_impact":0}`, "0"}},
		},
		{
			name:    "corrective action",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateCorrectiveAction },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				ca := &db.CorrectiveAction{Title: "ca", CreatedBy: nullCaseActor, DueDate: d(5)}
				applyCorrectiveActionDefaults(ca)
				if err := s.db.CreateCorrectiveAction(context.Background(), orgID, ca); err != nil {
					t.Fatalf("CreateCorrectiveAction: %v", err)
				}
				return ca.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetCorrectiveAction(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetCorrectiveAction: %v", err)
				}
				return map[string]string{"due_date": ptrStr(x.DueDate)}
			},
			dates: []string{"due_date"},
		},
		{
			name:    "incident",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateIncident },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				inc := &db.Incident{Title: "incident", Severity: "low", Status: "open",
					IncidentType: "incident", Source: "internal",
					AuthorityNotified: "not_required", SubjectsNotified: "not_required",
					Reporter: nullCaseActor, DetectedAt: db.NewEpoch(time.Now())}
				if err := s.db.CreateIncident(context.Background(), orgID, inc); err != nil {
					t.Fatalf("CreateIncident: %v", err)
				}
				// CreateIncident does not write the notification dates.
				if _, err := s.db.Pool().Exec(context.Background(),
					`UPDATE incidents SET authority_notified_at = $2, subjects_notified_at = $3 WHERE id = $1`,
					inc.ID, d(1).Time, d(2).Time); err != nil {
					t.Fatalf("seeding notification dates: %v", err)
				}
				return inc.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetIncident(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetIncident: %v", err)
				}
				return map[string]string{
					"authority_notified_at": ptrStr(x.AuthorityNotifiedAt),
					"subjects_notified_at":  ptrStr(x.SubjectsNotifiedAt),
				}
			},
			dates: []string{"authority_notified_at", "subjects_notified_at"},
		},
		{
			name:    "audit",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateAudit },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				a := &db.Audit{Title: "audit", AuditType: "internal", Status: "planned", PlannedDate: d(1), EndDate: d(2)}
				if err := s.db.CreateAudit(context.Background(), orgID, a); err != nil {
					t.Fatalf("CreateAudit: %v", err)
				}
				return int64(a.ID)
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetAudit(context.Background(), orgID, int(id))
				if err != nil {
					t.Fatalf("GetAudit: %v", err)
				}
				return map[string]string{"planned_date": ptrStr(x.PlannedDate), "end_date": ptrStr(x.EndDate)}
			},
			dates: []string{"planned_date", "end_date"},
		},
		{
			name:    "audit finding",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateAuditFinding },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				a := &db.Audit{Title: "finding audit", AuditType: "internal", Status: "planned"}
				if err := s.db.CreateAudit(context.Background(), orgID, a); err != nil {
					t.Fatalf("CreateAudit: %v", err)
				}
				f := &db.AuditFinding{AuditID: a.ID, FindingType: "observation", Title: "finding",
					Description: "d", Status: "open", DueDate: d(1)}
				if err := s.db.AddAuditFinding(context.Background(), orgID, f); err != nil {
					t.Fatalf("AddAuditFinding: %v", err)
				}
				return f.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetAuditFinding(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetAuditFinding: %v", err)
				}
				return map[string]string{"due_date": ptrStr(x.DueDate)}
			},
			dates: []string{"due_date"},
		},
		{
			name:    "task",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateTask },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				nullCaseUser(t, s, orgID, "assignee@null-clears.test", "contributor")
				task := &db.Task{Title: "task", TaskType: "general", Assignee: "assignee@null-clears.test",
					CreatedBy: nullCaseActor, Status: "open", Priority: "medium", DueDate: d(1), RecurrenceDays: i(30)}
				if err := s.db.CreateTask(context.Background(), orgID, task); err != nil {
					t.Fatalf("CreateTask: %v", err)
				}
				return task.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetTask(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetTask: %v", err)
				}
				return map[string]string{"due_date": ptrStr(x.DueDate), "recurrence_days": ptrStr(x.RecurrenceDays)}
			},
			dates: []string{"due_date"},
			zeros: []nullZero{
				{"recurrence_days", `{"recurrence_days":7}`, "7"},
				{"recurrence_days", `{"recurrence_days":0}`, "0"},
			},
		},
		{
			name:    "change request",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateChange },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				cr := &db.ChangeRequest{Title: "change", Description: "d", Priority: "medium", Category: "process",
					RiskLevel: "low", Status: "proposed", RequestedBy: nullCaseActor, PlannedAt: d(1)}
				if err := s.db.CreateChangeRequest(context.Background(), orgID, cr); err != nil {
					t.Fatalf("CreateChangeRequest: %v", err)
				}
				return int64(cr.ID)
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetChangeRequest(context.Background(), orgID, int(id))
				if err != nil {
					t.Fatalf("GetChangeRequest: %v", err)
				}
				return map[string]string{"planned_at": ptrStr(x.PlannedAt)}
			},
			dates: []string{"planned_at"},
		},
		{
			name:    "objective",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateObjective },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				p := &db.Program{Key: nullCaseProgramKey(), Title: "program"}
				if err := s.db.CreateProgram(context.Background(), orgID, p); err != nil {
					t.Fatalf("CreateProgram: %v", err)
				}
				target, window := 99.5, 3600
				o := &db.Objective{ProgramID: p.ID, Title: "objective", TargetValue: &target,
					WindowSeconds: &window, StartedAt: d(1)}
				if err := s.db.CreateObjective(context.Background(), orgID, o); err != nil {
					t.Fatalf("CreateObjective: %v", err)
				}
				return o.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetObjective(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetObjective: %v", err)
				}
				return map[string]string{
					"target_value": ptrStr(x.TargetValue), "window_seconds": ptrStr(x.WindowSeconds),
					"started_at": ptrStr(x.StartedAt),
				}
			},
			dates: []string{"started_at"},
			zeros: []nullZero{
				{"target_value", `{"target_value":0}`, "0"},
				{"window_seconds", `{"window_seconds":0}`, "0"},
			},
		},
		{
			name:    "checkin",
			handler: func(s *Server) func(echo.Context) error { return s.handleUpdateCheckin },
			seed: func(t *testing.T, s *Server, orgID int) int64 {
				p := &db.Program{Key: nullCaseProgramKey(), Title: "checkin program"}
				if err := s.db.CreateProgram(context.Background(), orgID, p); err != nil {
					t.Fatalf("CreateProgram: %v", err)
				}
				o := &db.Objective{ProgramID: p.ID, Title: "checkin objective"}
				if err := s.db.CreateObjective(context.Background(), orgID, o); err != nil {
					t.Fatalf("CreateObjective: %v", err)
				}
				ok, value := true, 7.5
				c := &db.Checkin{ObjectiveID: o.ID, Success: &ok, ValueNumeric: &value, CreatedBy: nullCaseActor}
				if err := s.db.CreateCheckin(context.Background(), orgID, c); err != nil {
					t.Fatalf("CreateCheckin: %v", err)
				}
				return c.ID
			},
			snapshot: func(t *testing.T, s *Server, orgID int, id int64) map[string]string {
				x, err := s.db.GetCheckin(context.Background(), orgID, id)
				if err != nil {
					t.Fatalf("GetCheckin: %v", err)
				}
				return map[string]string{"success": ptrStr(x.Success), "value_numeric": ptrStr(x.ValueNumeric)}
			},
			zeros: []nullZero{
				{"success", `{"success":false}`, "false"},
				{"value_numeric", `{"value_numeric":0}`, "0"},
			},
		},
	}
}

// putNullCase sends body to the entity's update handler and fails on anything
// but a 200.
func putNullCase(t *testing.T, s *Server, orgID int, ent nullEntity, id int64, body string) {
	t.Helper()
	c, rec := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", id), body)
	if err := ent.handler(s)(c); err != nil {
		t.Fatalf("PUT %s: %v", body, err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT %s: status = %d, want 200; body %s", body, rec.Code, rec.Body.String())
	}
}

func diffSnapshots(got, want map[string]string, skip map[string]bool) string {
	for k, w := range want {
		if skip[k] {
			continue
		}
		if got[k] != w {
			return fmt.Sprintf("%s = %s, want %s", k, got[k], w)
		}
	}
	return ""
}

func isDate(ent nullEntity, key string) bool {
	for _, d := range ent.dates {
		if d == key {
			return true
		}
	}
	return false
}

func TestNullClearsOptionalUpdateFields(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "null-clears")
	// The handlers write a changelog and activity row under this actor.
	nullCaseUser(t, s, orgID, nullCaseActor, "admin")

	for _, ent := range nullCases() {
		t.Run(ent.name, func(t *testing.T) {
			// Keys that null does not simply clear: next_review on the entities
			// that recalculate it, which has its own sub-test below.
			skip := map[string]bool{}
			if ent.recalcNextReview {
				skip["next_review"] = true
			}

			t.Run("empty body changes nothing", func(t *testing.T) {
				id := ent.seed(t, s, orgID)
				before := ent.snapshot(t, s, orgID, id)
				for k, v := range before {
					if v == "<nil>" {
						t.Fatalf("seed left %s nil; the case would prove nothing", k)
					}
				}
				putNullCase(t, s, orgID, ent, id, `{}`)
				// Any update recalculates next_review on these entities, as it
				// did before this change, so the date is not compared here.
				if d := diffSnapshots(ent.snapshot(t, s, orgID, id), before, skip); d != "" {
					t.Fatalf("after {}: %s", d)
				}
			})

			for _, mode := range []struct {
				name  string
				value string
				only  func(key string) bool
			}{
				{"null clears", "null", func(string) bool { return true }},
				{"empty string clears a date", `""`, func(k string) bool { return isDate(ent, k) }},
			} {
				t.Run(mode.name, func(t *testing.T) {
					id := ent.seed(t, s, orgID)
					current := ent.snapshot(t, s, orgID, id)
					for key := range current {
						if skip[key] || !mode.only(key) {
							continue
						}
						putNullCase(t, s, orgID, ent, id, fmt.Sprintf(`{%q:%s}`, key, mode.value))
						got := ent.snapshot(t, s, orgID, id)
						if got[key] != "<nil>" {
							t.Errorf("%s: %s = %s after %s, want it cleared", key, key, got[key], mode.value)
						}
						// Every other field must be exactly as it was before.
						current[key] = "<nil>"
						if d := diffSnapshots(got, current, skip); d != "" {
							t.Errorf("%s: clearing it changed a sibling: %s", key, d)
						}
					}
				})
			}

			if ent.recalcNextReview {
				t.Run("null next_review recalculates", func(t *testing.T) {
					id := ent.seed(t, s, orgID)
					// Pin a date no review cycle can produce, then send null.
					putNullCase(t, s, orgID, ent, id, fmt.Sprintf(`{"next_review":%q}`, explicitReviewDate))
					if got := ent.snapshot(t, s, orgID, id)["next_review"]; got[:10] != explicitReviewDate {
						t.Fatalf("setup: next_review = %s, want %s", got, explicitReviewDate)
					}
					putNullCase(t, s, orgID, ent, id, `{"next_review":null}`)
					got := ent.snapshot(t, s, orgID, id)["next_review"]
					if got == "<nil>" {
						t.Fatalf("next_review was cleared; null must recalculate it")
					}
					if got[:10] == explicitReviewDate {
						t.Fatalf("next_review is still the pinned %s; null must recalculate it", explicitReviewDate)
					}
				})
			}

			for _, z := range ent.zeros {
				t.Run("zero value is stored, not cleared: "+z.key, func(t *testing.T) {
					id := ent.seed(t, s, orgID)
					putNullCase(t, s, orgID, ent, id, z.body)
					if got := ent.snapshot(t, s, orgID, id)[z.key]; got != z.want {
						t.Fatalf("%s = %s after %s, want %s", z.key, got, z.body, z.want)
					}
				})
			}
		})
	}
}

// TestNullSupplierIDUnlinksSystem covers the visible symptom behind #381: the
// system edit form sends supplier_id: null for "No supplier" and the supplier
// stayed linked. It also checks that a real id still links.
func TestNullSupplierIDUnlinksSystem(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "null-clears-system-supplier")
	nullCaseUser(t, s, orgID, nullCaseActor, "admin")

	sup := &db.Supplier{Name: "linked supplier", SupplierType: "cloud", Criticality: "low", Status: "active"}
	if err := s.db.CreateSupplier(ctx, orgID, sup); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	sys := &db.System{Name: "linked system", Classification: "internal", Criticality: "low", Status: "active", SupplierID: &sup.ID}
	if err := s.db.CreateSystem(ctx, orgID, sys); err != nil {
		t.Fatalf("CreateSystem: %v", err)
	}
	ent := nullEntity{handler: func(s *Server) func(echo.Context) error { return s.handleUpdateSystem }}
	linked := func() string {
		got, err := s.db.GetSystem(ctx, orgID, sys.ID)
		if err != nil {
			t.Fatalf("GetSystem: %v", err)
		}
		return ptrStr(got.SupplierID)
	}

	putNullCase(t, s, orgID, ent, sys.ID, `{"supplier_id":null}`)
	if got := linked(); got != "<nil>" {
		t.Fatalf("supplier_id = %s after null, want it unlinked", got)
	}
	putNullCase(t, s, orgID, ent, sys.ID, fmt.Sprintf(`{"supplier_id":%d}`, sup.ID))
	if got, want := linked(), fmt.Sprint(sup.ID); got != want {
		t.Fatalf("supplier_id = %s after a real id, want %s", got, want)
	}

	// An id from another organization must still be refused.
	otherOrg := newTestOrg(t, s, "null-clears-system-other")
	other := &db.Supplier{Name: "foreign supplier", SupplierType: "cloud", Criticality: "low", Status: "active"}
	if err := s.db.CreateSupplier(ctx, otherOrg, other); err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	c, _ := ctxFor(orgID, http.MethodPut, fmt.Sprintf("%d", sys.ID), fmt.Sprintf(`{"supplier_id":%d}`, other.ID))
	err := s.handleUpdateSystem(c)
	if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
		t.Fatalf("supplier_id of another org: err = %v, want a 400", err)
	}
}
