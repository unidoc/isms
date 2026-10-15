package main

import (
	"errors"
	"testing"

	"isms.sh/internal/isms/refs"
)

func TestReferencesExit(t *testing.T) {
	cases := []struct {
		name string
		res  refs.CheckResult
		want int // 0 = nil error
	}{
		{"clean", refs.CheckResult{}, 0},
		{"broken, report only", refs.CheckResult{Broken: 2}, 1},
		{"unchecked", refs.CheckResult{Unchecked: 1}, 1},
		{"fix removed everything", refs.CheckResult{Broken: 2, Deleted: 2}, 0},
		{"fix removed all broken but one was unchecked", refs.CheckResult{Broken: 2, Deleted: 2, Unchecked: 1}, 1},
		{"fix removed some", refs.CheckResult{Broken: 3, Deleted: 2}, 1},
		{"non-canonical, report only", refs.CheckResult{NonCanonical: 2}, 1},
		{"comment mentions only, report only", refs.CheckResult{NonCanonicalComments: 1}, 1},
		{"non-canonical all fixed", refs.CheckResult{NonCanonical: 2, Fixed: 2}, 0},
		{"comment mentions all fixed", refs.CheckResult{NonCanonicalComments: 2, FixedComments: 2}, 0},
		{"broken deleted but non-canonical left", refs.CheckResult{Broken: 1, Deleted: 1, NonCanonical: 1}, 1},
	}
	for _, c := range cases {
		err := referencesExit(c.res)
		var ee *exitError
		switch {
		case c.want == 0 && err != nil:
			t.Errorf("%s: err = %v, want nil", c.name, err)
		case c.want != 0 && (!errors.As(err, &ee) || ee.code != c.want):
			t.Errorf("%s: err = %v, want exit %d", c.name, err, c.want)
		}
	}
}
