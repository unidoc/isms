package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The API error seam sends `code` and `params` beside the English `message` so a
// browser can render the sentence in the reader's own locale. On a terminal they
// are noise, and this file pins that the CLI shows the sentence rather than the
// envelope — plus, more importantly, that a body which is *not* that envelope
// still reaches the operator verbatim.
func TestErrorDetail(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{{
		name: "a converted API error shows the message, not the envelope",
		body: `{"message":"risk not found","code":"not_found","params":{"entity":"risk"}}`,
		want: "risk not found",
	}, {
		name: "an unconverted API error is unchanged by the seam",
		body: `{"message":"review is not in changes_requested state"}`,
		want: "review is not in changes_requested state",
	}, {
		// Roughly 520 sites still answer with a bare message on purpose — 5xx
		// wraps and operator configuration text. They must read the same as
		// before.
		name: "an operator configuration message survives intact",
		body: `{"message":"WebAuthn not configured (set ISMS_BASE_URL)"}`,
		want: "WebAuthn not configured (set ISMS_BASE_URL)",
	}, {
		// The half that matters most: a 4xx does not always come from the API.
		name: "a proxy's HTML page passes through verbatim",
		body: "<html><head><title>502 Bad Gateway</title></head></html>",
		want: "<html><head><title>502 Bad Gateway</title></head></html>",
	}, {
		name: "plain text passes through, trimmed",
		body: "  access denied by policy\n",
		want: "access denied by policy",
	}, {
		name: "an empty body says so rather than trailing a colon into nothing",
		body: "",
		want: "(no response body)",
	}, {
		name: "whitespace only is an empty body",
		body: "   \n\t ",
		want: "(no response body)",
	}, {
		// A body carrying a code but no message is not the documented shape.
		// Showing it raw is the honest answer: something is wrong with the
		// response itself, and hiding it would hide that.
		name: "a code with no message falls back to the raw body",
		body: `{"code":"not_found"}`,
		want: `{"code":"not_found"}`,
	}, {
		name: "an explicitly empty message falls back to the raw body",
		body: `{"message":""}`,
		want: `{"message":""}`,
	}, {
		// A whitespace-only message is no message. Our own server cannot send
		// one, but the fallback exists for bodies that are not ours, and
		// "API error 502:   " tells an operator less than the body did.
		name: "a whitespace-only message falls back to the raw body",
		body: `{"message":"   "}`,
		want: `{"message":"   "}`,
	}, {
		name: "a newline-only message falls back to the raw body",
		body: `{"message":"\n"}`,
		want: `{"message":"\n"}`,
	}, {
		// The decoded message is trimmed for the same reason the fallback is:
		// a stray newline in the envelope should not break the caller's line.
		name: "surrounding whitespace is trimmed off a real message",
		body: `{"message":"  risk not found\n"}`,
		want: "risk not found",
	}, {
		// json.Unmarshal into a struct fails on a non-object, so this reaches
		// the fallback rather than silently yielding "".
		name: "a JSON array is not an error envelope",
		body: `["nope"]`,
		want: `["nope"]`,
	}, {
		name: "malformed JSON passes through so the operator can see it",
		body: `{"message":"truncated`,
		want: `{"message":"truncated`,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorDetail([]byte(tc.body)); got != tc.want {
				t.Errorf("errorDetail(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// errorDetail being correct proves nothing if do() does not call it, which is
// exactly the shape of the defect this fixes: the helper the docs asked for was
// never wired in. So this drives the real request path.
func TestDoRendersTheServersMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"risk not found","code":"not_found","params":{"entity":"risk"}}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	_, err := c.get("/risks/999")
	if err == nil {
		t.Fatal("expected an error for a 404")
	}

	want := "API error 404: risk not found"
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
	// The regression, stated as an assertion rather than a comment: the wire
	// fields must not reach the terminal.
	for _, leaked := range []string{"code", "params", "not_found", "{"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("error text leaks the wire envelope (%q): %q", leaked, err.Error())
		}
	}
}

func TestDoPassesThroughANonAPIErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><title>502 Bad Gateway</title></html>"))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL})
	_, err := c.get("/risks")
	if err == nil {
		t.Fatal("expected an error for a 502")
	}
	if want := "API error 502: <html><title>502 Bad Gateway</title></html>"; err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}
