package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"isms.sh/internal/isms/db"
)

// Requires a migrated Postgres; skipped otherwise:
//
//	ISMS_TEST_DATABASE_URL="postgres://…/isms_db_test?sslmode=disable" go test ./internal/isms/api/ -run UTCDates
//
// #503: a calendar date (a DATE column) must stay on its UTC day on a server
// whose host zone is not UTC. CI runs in UTC, so these tests set time.Local
// themselves. Never parallel.

func setHostZone(t *testing.T, offsetSeconds int) {
	t.Helper()
	prev := time.Local
	time.Local = time.FixedZone("host", offsetSeconds)
	t.Cleanup(func() { time.Local = prev })
}

// taskCall drives one task handler. param is "" for create.
func taskCall(t *testing.T, orgID int, handler func(echo.Context) error, method, param, body string) (int, []byte) {
	t.Helper()
	e := echo.New()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "/api/v1/tasks", nil)
	} else {
		req = httptest.NewRequest(method, "/api/v1/tasks", strings.NewReader(body))
	}
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if param != "" {
		c.SetParamNames("id")
		c.SetParamValues(param)
	}
	c.Set("org_id", orgID)
	c.Set("user_role", "admin")
	c.Set("user_email", "admin@utc-dates.test")
	if err := handler(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code, []byte(fmt.Sprint(he.Message))
		}
		t.Fatalf("handler: %v", err)
	}
	return rec.Code, rec.Body.Bytes()
}

func TestUTCDatesTaskDueDateRoundTrip(t *testing.T) {
	s := testServer(t)
	orgID := newTestOrg(t, s, "utc-dates-task")
	contractTestUser(t, s, orgID, "admin@utc-dates.test", "admin")

	for _, zone := range []struct {
		name   string
		offset int
	}{{"west", -5 * 3600}, {"east", 7 * 3600}} {
		for _, in := range []struct{ name, due string }{
			{"epoch", `1767225600`},
			{"string", `"2026-01-01"`}, // control: always worked
		} {
			t.Run(zone.name+"/"+in.name, func(t *testing.T) {
				setHostZone(t, zone.offset)
				code, body := taskCall(t, orgID, s.handleCreateTask, http.MethodPost, "",
					fmt.Sprintf(`{"title":"tz %s %s","due_date":%s}`, zone.name, in.name, in.due))
				if code != http.StatusCreated && code != http.StatusOK {
					t.Fatalf("create = %d: %s", code, body)
				}
				var created struct {
					ID int64 `json:"id"`
				}
				if err := json.Unmarshal(body, &created); err != nil || created.ID == 0 {
					t.Fatalf("create body %s: %v", body, err)
				}
				id := fmt.Sprint(created.ID)

				read := func() (int64, string) {
					code, body := taskCall(t, orgID, s.handleGetTask, http.MethodGet, id, "")
					if code != http.StatusOK {
						t.Fatalf("get = %d: %s", code, body)
					}
					var got struct {
						DueDate *int64 `json:"due_date"`
					}
					if err := json.Unmarshal(body, &got); err != nil || got.DueDate == nil {
						t.Fatalf("get body %s: %v", body, err)
					}
					return *got.DueDate, time.Unix(*got.DueDate, 0).UTC().Format("2006-01-02")
				}
				epoch, day := read()
				if day != "2026-01-01" || epoch != 1767225600 {
					t.Fatalf("after create: due_date %d = %s, want 1767225600 = 2026-01-01", epoch, day)
				}
				// Round trip: send the value read back, unchanged, twice. It used
				// to drift one day further back on every save.
				for i := 1; i <= 2; i++ {
					code, body := taskCall(t, orgID, s.handleUpdateTask, http.MethodPut, id,
						fmt.Sprintf(`{"due_date":%d}`, epoch))
					if code != http.StatusOK {
						t.Fatalf("update %d = %d: %s", i, code, body)
					}
					if e2, d2 := read(); e2 != 1767225600 {
						t.Fatalf("after save %d: due_date %d = %s, want 2026-01-01", i, e2, d2)
					}
				}
			})
		}
	}
}

// hostZoneOffDay picks a host zone whose calendar day is not the UTC day right
// now, so the stamp test is meaningful whenever it runs: UTC-12 differs while
// the UTC hour is below 12, UTC+14 differs from hour 10 on. No clock seam
// exists and none is added.
func hostZoneOffDay() int {
	if time.Now().UTC().Hour() < 12 {
		return -12 * 3600
	}
	return 14 * 3600
}

func TestUTCDatesReadingStampsUTCToday(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	orgID := newTestOrg(t, s, "utc-dates-stamp")
	contractTestUser(t, s, orgID, "admin@utc-dates.test", "admin")

	likelihood, impact := 2, 3
	risk := &db.Risk{Title: "stamp", RiskType: db.RiskTypes[0], Origin: db.RiskOrigins[0], Status: db.RiskStatuses[0],
		Treatment: "mitigate", CurrentLikelihood: &likelihood, CurrentImpact: &impact}
	if err := s.db.CreateRisk(ctx, orgID, risk); err != nil {
		t.Fatalf("CreateRisk: %v", err)
	}

	setHostZone(t, hostZoneOffDay())
	if time.Now().Format("2006-01-02") == time.Now().UTC().Format("2006-01-02") {
		t.Fatal("test setup: local and UTC day are equal")
	}
	utcToday := time.Now().UTC().Format("2006-01-02")

	c, rec := ctxFor(orgID, http.MethodPost, fmt.Sprintf("%d", risk.ID), `{"current_likelihood":3,"current_impact":3}`)
	if err := s.handleCreateRiskReading(c); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got, err := s.db.GetRisk(ctx, orgID, risk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if utcDate(got.LastReview) != utcToday {
		t.Errorf("last_review = %s, want the UTC day %s", utcDate(got.LastReview), utcToday)
	}
}
