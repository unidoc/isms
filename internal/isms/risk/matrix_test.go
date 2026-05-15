package risk

import (
	"strings"
	"testing"
)

func TestLevelChar(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		// Low: 1-4
		{0, "L"},
		{1, "L"},
		{2, "L"},
		{3, "L"},
		{4, "L"},
		// Medium: 5-9
		{5, "M"},
		{6, "M"},
		{8, "M"},
		{9, "M"},
		// High: 10-15
		{10, "H"},
		{12, "H"},
		{15, "H"},
		// Critical: 16-25
		{16, "C"},
		{20, "C"},
		{25, "C"},
	}

	for _, tt := range tests {
		got := levelChar(tt.score)
		if got != tt.want {
			t.Errorf("levelChar(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestLevelChar_BoundaryValues(t *testing.T) {
	// Test exact boundaries between levels
	tests := []struct {
		name  string
		score int
		want  string
	}{
		{"boundary low/medium at 4", 4, "L"},
		{"boundary low/medium at 5", 5, "M"},
		{"boundary medium/high at 9", 9, "M"},
		{"boundary medium/high at 10", 10, "H"},
		{"boundary high/critical at 15", 15, "H"},
		{"boundary high/critical at 16", 16, "C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := levelChar(tt.score)
			if got != tt.want {
				t.Errorf("levelChar(%d) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}

func TestLevelChar_NegativeScore(t *testing.T) {
	// Negative scores should fall into Low (the default case)
	got := levelChar(-1)
	if got != "L" {
		t.Errorf("levelChar(-1) = %q, want %q", got, "L")
	}
}

func TestRiskScore_LikelihoodTimesImpact(t *testing.T) {
	// Verify that the 5x5 matrix produces correct scores (likelihood * impact)
	// and that each score maps to the expected risk level.
	tests := []struct {
		likelihood int
		impact     int
		wantScore  int
		wantLevel  string
	}{
		{1, 1, 1, "L"},
		{1, 2, 2, "L"},
		{1, 3, 3, "L"},
		{1, 4, 4, "L"},
		{1, 5, 5, "M"},
		{2, 1, 2, "L"},
		{2, 2, 4, "L"},
		{2, 3, 6, "M"},
		{2, 4, 8, "M"},
		{2, 5, 10, "H"},
		{3, 3, 9, "M"},
		{3, 4, 12, "H"},
		{3, 5, 15, "H"},
		{4, 4, 16, "C"},
		{4, 5, 20, "C"},
		{5, 1, 5, "M"},
		{5, 2, 10, "H"},
		{5, 3, 15, "H"},
		{5, 4, 20, "C"},
		{5, 5, 25, "C"},
	}

	for _, tt := range tests {
		score := tt.likelihood * tt.impact
		if score != tt.wantScore {
			t.Errorf("likelihood=%d * impact=%d = %d, want %d", tt.likelihood, tt.impact, score, tt.wantScore)
		}
		level := levelChar(score)
		if level != tt.wantLevel {
			t.Errorf("levelChar(%d) for L=%d I=%d = %q, want %q", score, tt.likelihood, tt.impact, level, tt.wantLevel)
		}
	}
}

func TestRiskScore_EdgeCases(t *testing.T) {
	// Zero likelihood or impact
	if score := 0 * 5; levelChar(score) != "L" {
		t.Errorf("zero likelihood should give Low, got %s", levelChar(score))
	}
	if score := 5 * 0; levelChar(score) != "L" {
		t.Errorf("zero impact should give Low, got %s", levelChar(score))
	}
	if score := 0 * 0; levelChar(score) != "L" {
		t.Errorf("zero both should give Low, got %s", levelChar(score))
	}

	// Max values
	if score := 5 * 5; levelChar(score) != "C" {
		t.Errorf("max score should give Critical, got %s", levelChar(score))
	}

	// Beyond normal range
	if score := 10 * 10; levelChar(score) != "C" {
		t.Errorf("above-max score should give Critical, got %s", levelChar(score))
	}
}

func TestPrintMatrix(t *testing.T) {
	output := PrintMatrix()

	// Should contain the header (uses Unicode multiplication sign)
	if !strings.Contains(output, "5\u00d75 Risk Matrix") {
		t.Error("PrintMatrix output should contain risk matrix header")
	}

	// Should contain the legend
	if !strings.Contains(output, "L = Low") {
		t.Error("PrintMatrix output should contain Low legend")
	}
	if !strings.Contains(output, "M = Medium") {
		t.Error("PrintMatrix output should contain Medium legend")
	}
	if !strings.Contains(output, "H = High") {
		t.Error("PrintMatrix output should contain High legend")
	}
	if !strings.Contains(output, "C = Critical") {
		t.Error("PrintMatrix output should contain Critical legend")
	}

	// Should contain all 25 cells (scores from the 5x5 matrix)
	// Check for a few specific score/level combos
	if !strings.Contains(output, "1(L)") {
		t.Error("PrintMatrix should contain 1(L)")
	}
	if !strings.Contains(output, "25(C)") {
		t.Error("PrintMatrix should contain 25(C)")
	}
	if !strings.Contains(output, "10(H)") {
		t.Error("PrintMatrix should contain 10(H)")
	}

	// Should have 5 data rows (likelihood 5 down to 1)
	lines := strings.Split(output, "\n")
	dataRows := 0
	for _, line := range lines {
		// Data rows contain score(level) patterns like "12(H)"
		if strings.Count(line, "(") >= 5 {
			dataRows++
		}
	}
	if dataRows != 5 {
		t.Errorf("expected 5 data rows, got %d", dataRows)
	}
}
