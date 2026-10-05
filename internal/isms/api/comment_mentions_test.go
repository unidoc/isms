package api

import (
	"reflect"
	"testing"
)

// The fixtures mirror web/test/entityReferenceLinks.test.js: a mention is
// saved as a link (#194) only if the page renders it as one, so the two
// parsers must agree on what is a reference.
func TestParseEntityMentions(t *testing.T) {
	cases := []struct {
		body string
		want []entityMention
	}{
		{"see #RISK-1", []entityMention{{"risk", "RISK-1"}}},
		{"#CA-12 and #CR-3", []entityMention{{"corrective_action", "CA-12"}, {"change_request", "CR-3"}}},
		{"#AST-2 #ASSET-3", []entityMention{{"asset", "AST-2"}, {"asset", "ASSET-3"}}},
		{"#ISMS-4", []entityMention{{"objective", "ISMS-4"}}},
		{"#ISMS", []entityMention{{"program", "ISMS"}}},
		{"#FOO-1", []entityMention{{"objective", "FOO-1"}}}, // resolves only if FOO is a program key
		{"issue #42 and #lowercase-1", nil},
		{"#RISK-1x", []entityMention{{"program", "RISK"}}}, // no word boundary after 1, as in the page
		{"#RISK-1, again #RISK-1.", []entityMention{{"risk", "RISK-1"}}},
		{"no mentions here", nil},
	}
	for _, tc := range cases {
		if got := parseEntityMentions(tc.body); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseEntityMentions(%q) = %v, want %v", tc.body, got, tc.want)
		}
	}
}
