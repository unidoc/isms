package api

import "testing"

// Browsers and the web helper accept only 3, 4, 6 and 8 digit hex colors; the
// API must not store 5 or 7 digit values that silently render as the default (#408).
func TestValidBrandingColor(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"#abc", true},
		{"#abcd", true},
		{"#aabbcc", true},
		{"#aabbccdd", true},
		{"#ABC", true},
		{"#AABBCCDD", true},
		{"#1a365d", true},
		{"#12345", false},
		{"#1234567", false},
		{"#abcde12345", false},
		{"aabbcc", false},
		{"#ggg", false},
		{"#aabbccd", false},
		{"#ab", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := validBrandingColor(tc.in); got != tc.want {
			t.Errorf("validBrandingColor(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
