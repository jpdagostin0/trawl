// Package answer computes instant answers (calculator, unit conversion) shown
// above the results, like the old Google calculator onebox.
package answer

import (
	"math"
	"strconv"
	"strings"
)

type Answer struct {
	Kind   string `json:"kind"`   // "calc" or "convert"
	Expr   string `json:"expr"`   // the question, normalized: "5 + 5", "10 kilometers"
	Result string `json:"result"` // "10", "6.21371 miles"
}

// For returns an instant answer for q, or nil.
func For(q string) *Answer {
	q = strings.TrimSpace(q)
	if len(q) == 0 || len(q) > 200 {
		return nil
	}
	if a := convert(q); a != nil {
		return a
	}
	return calc(q)
}

// formatNum prints a float the way the old calculator did: integers in full,
// otherwise 12 significant digits, large/small values as "1.5 x 10^20".
func formatNum(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ""
	}
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	s := strconv.FormatFloat(v, 'g', 12, 64)
	if mant, exp, ok := strings.Cut(s, "e"); ok {
		n, _ := strconv.Atoi(exp)
		return mant + " x 10^" + strconv.Itoa(n)
	}
	return s
}
