package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
	"isms.sh/internal/isms/db"
)

// A db.ValidationError — even wrapped, as suggestion-apply wraps it — maps to
// 400 with the unwrapped message (#296).
func TestPgxHTTPErrorValidationError(t *testing.T) {
	bad := 9
	verr := (&db.Risk{Title: "t", RiskType: "threat", Origin: "internal", Status: "open", CurrentLikelihood: &bad}).Validate()
	for name, in := range map[string]error{
		"bare":    verr,
		"wrapped": fmt.Errorf("apply: %w", verr),
	} {
		t.Run(name, func(t *testing.T) {
			var he *echo.HTTPError
			if !errors.As(pgxHTTPError(in), &he) {
				t.Fatalf("pgxHTTPError did not return *echo.HTTPError")
			}
			if he.Code != http.StatusBadRequest {
				t.Fatalf("code = %d, want 400", he.Code)
			}
			if he.Message != "current_likelihood must be 0-5" {
				t.Fatalf("message = %v, want %q", he.Message, "current_likelihood must be 0-5")
			}
		})
	}
}

// A plain error is still a 500 — the new branch must not swallow real failures.
func TestPgxHTTPErrorPlainErrorStays500(t *testing.T) {
	var he *echo.HTTPError
	if !errors.As(pgxHTTPError(errors.New("boom")), &he) || he.Code != http.StatusInternalServerError {
		t.Fatalf("plain error should map to 500, got %+v", he)
	}
}
