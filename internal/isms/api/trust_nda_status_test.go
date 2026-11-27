package api

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

// A failed NDA status lookup after a side effect is logged and answered as
// "unavailable" by the visitor responses and the block answer; with no error the
// value passes through. (The failure itself cannot be forced without mutating the
// shared test database, so the decision is the unit.)
func TestTrustNDAOrUnavailable(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(prev)

	got := &trustVisitorNDA{Status: trustNDAAccepted, Version: "2"}
	fallback := &trustVisitorNDA{Status: trustNDAUnavailable}
	if out := trustNDAOrUnavailable(got, fallback, nil, "visitor 1"); out != got {
		t.Errorf("no error: got %+v, want the value unchanged", out)
	}
	if buf.Len() != 0 {
		t.Errorf("no error logged something: %s", buf.String())
	}
	if out := trustNDAOrUnavailable(got, fallback, errors.New("relation is gone"), "org 3 visitor 9"); out != fallback || out.Status != "unavailable" {
		t.Errorf("error: got %+v, want the unavailable fallback", out)
	}
	if l := buf.String(); !strings.Contains(l, "org 3 visitor 9") || !strings.Contains(l, "relation is gone") {
		t.Errorf("the failure was not logged with its context: %q", l)
	}

	// The manager's row type goes through the same helper.
	row := trustManagerVisitorNDA{Status: trustNDARequired, Version: "2"}
	if out := trustNDAOrUnavailable(row, trustManagerVisitorNDA{Status: trustNDAUnavailable}, errors.New("x"), "visitor 4"); out.Status != "unavailable" || out.Version != "" {
		t.Errorf("manager row on error = %+v", out)
	}
	if out := trustNDAOrUnavailable(row, trustManagerVisitorNDA{Status: trustNDAUnavailable}, nil, "visitor 4"); out != row {
		t.Errorf("manager row without error = %+v", out)
	}
}
