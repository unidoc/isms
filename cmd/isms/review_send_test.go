package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// replyServer answers every request with the given status and body, and records
// the method and path of the last one. Like cliServer it points the CLI at itself.
func replyServer(t *testing.T, status int, body string) *recordedReq {
	t.Helper()
	got := &recordedReq{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ISMS_API_URL", srv.URL)
	t.Setenv("ISMS_API_KEY", "test-token")
	return got
}

// captureStdout runs fn and returns what it printed to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestReviewSendPrintsEachOutcome(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		args   []string
		want   []string
		absent []string
	}{
		{
			name:   "created",
			status: http.StatusCreated,
			body:   `{"review_id":7,"version":"1.0"}`,
			args:   []string{"pol-1", "--to", "a@x.io", "--to", "b@x.io"},
			want:   []string{"Review #7 created for pol-1 (v1.0)", "Reviewers: a@x.io, b@x.io"},
		},
		{
			name:   "resubmitted",
			status: http.StatusOK,
			body:   `{"review_id":7,"version":"1.1","round":2,"message":"review resubmitted with changes"}`,
			args:   []string{"pol-1"},
			want:   []string{"Review #7 for pol-1 resubmitted (round 2, v1.1)"},
			absent: []string{"created", "Reviewers", "No reviewers assigned"},
		},
		{
			name:   "resubmitted with new reviewers",
			status: http.StatusOK,
			body:   `{"review_id":7,"version":"1.1","round":2,"message":"review resubmitted with changes"}`,
			args:   []string{"pol-1", "--to", "c@x.io"},
			want:   []string{"resubmitted (round 2, v1.1)", "Reviewers added: c@x.io"},
		},
		{
			name:   "no version",
			status: http.StatusCreated,
			body:   `{"review_id":4,"version":""}`,
			args:   []string{"iso-1", "--to", "a@x.io"},
			want:   []string{"Review #4 created for iso-1\n"},
			absent: []string{"(v", "()"},
		},
		{
			name:   "resubmitted no version",
			status: http.StatusOK,
			body:   `{"review_id":1,"version":"","round":2,"message":"review resubmitted with changes"}`,
			args:   []string{"iso-1"},
			want:   []string{"Review #1 for iso-1 resubmitted (round 2)\n"},
			absent: []string{"v)", ", )"},
		},
		{
			name:   "reviewers added",
			status: http.StatusOK,
			body:   `{"review_id":7,"version":"1.1","message":"reviewers added to existing review"}`,
			args:   []string{"pol-1", "--to", "c@x.io"},
			want:   []string{"Review #7 for pol-1 is already open: reviewers added to existing review", "Reviewers: c@x.io"},
			absent: []string{"created", "resubmitted"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := replyServer(t, tc.status, tc.body)
			cmd := reviewSendCmd()
			cmd.SetArgs(tc.args)
			var err error
			out := captureStdout(t, func() { err = cmd.Execute() })
			if err != nil {
				t.Fatalf("review send: %v", err)
			}
			if want := "/v1/documents/" + tc.args[0] + "/reviews"; got.method != http.MethodPost || got.path != want {
				t.Errorf("request = %s %s, want POST %s", got.method, got.path, want)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output %q lacks %q", out, w)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(out, a) {
					t.Errorf("output %q should not contain %q", out, a)
				}
			}
		})
	}
}
