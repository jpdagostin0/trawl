package answer

import (
	"cmp"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"

	"gonum.org/v1/gonum/integrate/quad"
)

var (
	integralPrefix = regexp.MustCompile(`(?i)^(?:(?:what\s+is|what's|whats|calculate|compute|find|solve)\s+)?(?:the\s+)?(?:(?:integrate|integral(?:\s+of)?|int)\b|∫)\s*`)
	// "x^2 dx from 0 to 1", "x over 1, 5", "sin(t) on [0, pi] dt", "1/x between 1 and e"
	integralBody = regexp.MustCompile(`(?i)^(.+?)\s*(?:\bd([a-z]))?\s+(?:over|from|on|between)\s+[\[(]?\s*(.+?)\s*(?:,|\bto\b|\band\b|\.\.)\s*(.+?)\s*[\])]?\s*(?:\bd([a-z]))?$`)
	// "3x^2" and "2(x+1)": implicit multiplication, but not the 2 in log2(x)
	numTimes   = regexp.MustCompile(`(^|[^a-zA-Z_\d.])(\d+(?:\.\d+)?)\s*([a-zA-Z(])`)
	parenTimes = regexp.MustCompile(`\)\s*([a-zA-Z(\d])`)
	infinity   = regexp.MustCompile(`(?i)^([+-]?)\s*(?:inf|infinity|∞)$`)
)

// integral answers definite integrals numerically ("integrate x over 1, 5").
func integral(q string) *Answer {
	s := strings.TrimSpace(strings.TrimRight(q, "=? "))
	rest := integralPrefix.ReplaceAllString(s, "")
	if rest == s {
		return nil
	}
	m := integralBody.FindStringSubmatch(symbols.Replace(rest))
	if m == nil {
		return nil
	}
	fs := numTimes.ReplaceAllString(parenTimes.ReplaceAllString(m[1], ")*$1"), "$1$2*$3")
	lo, ok1 := bound(m[3])
	hi, ok2 := bound(m[4])
	if !ok1 || !ok2 {
		return nil
	}

	free := map[string]bool{}
	tree, _, ok := parseArith(fs, free)
	if !ok {
		return nil
	}
	v := cmp.Or(m[2], m[5])
	switch {
	case len(free) > 1:
		return nil
	case len(free) == 1 && v == "":
		v = slices.Collect(maps.Keys(free))[0]
		if len(v) != 1 { // a word, not a variable: "integral of cost from 1 to 5"
			return nil
		}
	case len(free) == 1 && !free[v]:
		return nil
	case v == "":
		v = "x"
	}
	prog, err := compileArith(fs, v)
	if err != nil {
		return nil
	}
	env := maps.Clone(mathEnv)
	failed := false
	f := func(x float64) float64 {
		env[v] = x
		y, ok := runFloat(prog, env)
		if !ok {
			failed = true
		}
		return y
	}

	a, b, sign := lo, hi, 1.0
	if a > b {
		a, b, sign = b, a, -1
	}
	// Gauss-Legendre (with a change of variables for infinite bounds) at two
	// orders: if they disagree a lot the integral is singular or divergent,
	// otherwise their agreement says how many digits to show.
	coarse := quad.Fixed(f, a, b, 200, nil, 0)
	fine := quad.Fixed(f, a, b, 400, nil, 0)
	if failed || math.IsNaN(fine) || math.IsInf(fine, 0) {
		return nil
	}
	digits := 12
	if diff := math.Abs(fine - coarse); diff > 0 {
		rel := diff / max(math.Abs(fine), 1e-300)
		if rel > 1e-4 && diff > 1e-9 {
			return nil
		}
		digits = min(12, max(4, int(-math.Log10(rel))))
	}
	res := sign * fine
	if math.Abs(res) < 1e-10 {
		res = 0
	}
	return &Answer{
		Kind:   "integral",
		Expr:   "∫ " + tree.Node.String() + " d" + v + " from " + strings.TrimSpace(m[3]) + " to " + strings.TrimSpace(m[4]),
		Result: formatSig(res, digits),
	}
}

// bound evaluates an integration limit: arithmetic ("pi/2", "1"), or ±infinity.
func bound(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if m := infinity.FindStringSubmatch(s); m != nil {
		if m[1] == "-" {
			return math.Inf(-1), true
		}
		return math.Inf(1), true
	}
	if _, _, ok := parseArith(s, nil); !ok {
		return 0, false
	}
	prog, err := compileArith(s)
	if err != nil {
		return 0, false
	}
	v, ok := runFloat(prog, mathEnv)
	return v, ok && !math.IsNaN(v)
}
