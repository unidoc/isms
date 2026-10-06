package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

// The request log replaced echo's deprecated middleware.Logger. Its line is
// parsed by whatever reads the server's stdout, so these pin the line Logger
// wrote: the field order, the error handling before the status is read, and
// the escaping of quotes while &, < and > stay readable.

// serveLogged runs one request through requestLogger and returns the response
// recorder and the single log line it wrote.
func serveLogged(t *testing.T, r *http.Request, h echo.HandlerFunc) (*httptest.ResponseRecorder, string) {
	t.Helper()
	e := echo.New()
	var out bytes.Buffer
	e.Logger.SetOutput(&out)
	e.Use(requestLogger())
	e.GET("/x", h)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	line := strings.TrimSuffix(out.String(), "\n")
	if line == "" || strings.Contains(line, "\n") {
		t.Fatalf("want exactly one log line, got %q", out.String())
	}
	return rec, line
}

// timing blanks the fields that change run to run.
var timing = regexp.MustCompile(`"time":"[^"]*"|"latency":\d+|"latency_human":"[^"]*"`)

func TestRequestLogLineMatchesEchoLoggerFormat(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?a=1&b=<x>", nil)
	r.Host = "acme.example.com"
	r.RemoteAddr = "203.0.113.9:4444"
	r.Header.Set("X-Request-Id", "req-123")
	r.Header.Set("User-Agent", `agent "quoted" & <tag>`)
	_, line := serveLogged(t, r, func(c echo.Context) error { return c.String(http.StatusOK, "fine") })

	want := `{"time":"","id":"req-123","remote_ip":"203.0.113.9","host":"acme.example.com",` +
		`"method":"GET","uri":"/x?a=1&b=<x>","user_agent":"agent \"quoted\" & <tag>",` +
		`"status":200,"error":"","latency":0,"latency_human":"","bytes_in":0,"bytes_out":4}`
	got := timing.ReplaceAllStringFunc(line, func(m string) string {
		if strings.HasPrefix(m, `"latency":`) {
			return `"latency":0`
		}
		return m[:strings.Index(m, ":")+1] + `""`
	})
	if got != want {
		t.Errorf("log line\n got: %s\nwant: %s", got, want)
	}
	if !json.Valid([]byte(line)) {
		t.Errorf("log line is not valid JSON: %s", line)
	}
}

// A handler error is turned into its response before the line is written, so
// the logged status is the one the client got, and the response is written
// once even though RequestLogger returns the handled error up the chain.
func TestRequestLogHandlesErrorOnce(t *testing.T) {
	rec, line := serveLogged(t, httptest.NewRequest("GET", "/x", nil), func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusTeapot, `te"apot`)
	})
	if rec.Code != http.StatusTeapot {
		t.Errorf("response status = %d, want 418", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"message":"te\"apot"}` {
		t.Errorf("response body = %s, want a single error body", body)
	}
	var got requestLogLine
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, line)
	}
	if got.Status != http.StatusTeapot || got.Error != `code=418, message=te"apot` {
		t.Errorf("logged status=%d error=%q, want 418 and the handler's error", got.Status, got.Error)
	}
}
