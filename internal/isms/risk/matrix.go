// Package risk provides risk assessment utilities.
package risk

import (
	"fmt"
	"strings"
)

// PrintMatrix renders a 5×5 risk heatmap to the terminal.
func PrintMatrix() string {
	var b strings.Builder

	b.WriteString("\n  5×5 Risk Matrix (Likelihood × Impact)\n\n")
	b.WriteString(fmt.Sprintf("  %-12s", "Impact →"))
	for i := 1; i <= 5; i++ {
		b.WriteString(fmt.Sprintf("  %6d", i))
	}
	b.WriteString("\n  Likelihood\n")

	for l := 5; l >= 1; l-- {
		b.WriteString(fmt.Sprintf("  %6d      ", l))
		for i := 1; i <= 5; i++ {
			score := l * i
			level := levelChar(score)
			b.WriteString(fmt.Sprintf("  %2d(%s)", score, level))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n  L = Low (1-4)  M = Medium (5-9)  H = High (10-15)  C = Critical (16-25)\n")
	return b.String()
}

func levelChar(score int) string {
	switch {
	case score >= 16:
		return "C"
	case score >= 10:
		return "H"
	case score >= 5:
		return "M"
	default:
		return "L"
	}
}
