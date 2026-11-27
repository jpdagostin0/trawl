package answer

import (
	"math"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// mathEnv is everything a calculator expression may reference.
var mathEnv = map[string]any{
	"pi": math.Pi, "e": math.E, "phi": math.Phi, "tau": 2 * math.Pi,
	"sqrt": math.Sqrt, "cbrt": math.Cbrt, "abs": math.Abs, "exp": math.Exp,
	"ln": math.Log, "log": math.Log10, "log10": math.Log10, "log2": math.Log2,
	"sin": math.Sin, "cos": math.Cos, "tan": math.Tan,
	"asin": math.Asin, "acos": math.Acos, "atan": math.Atan,
	"sinh": math.Sinh, "cosh": math.Cosh, "tanh": math.Tanh,
	"floor": math.Floor, "ceil": math.Ceil, "round": math.Round,
	"mod": math.Mod, "pow": math.Pow,
}

var (
	calcPrefix = regexp.MustCompile(`(?i)^(what\s+is|what's|whats|calculate|calc|compute|eval|solve)\s+`)
	// "5 x 5": x between numbers means times
	timesX  = regexp.MustCompile(`(\d|\))\s*[xX]\s*(\d|\()`)
	isoDate = regexp.MustCompile(`^\d{4}-\d{1,2}-\d{1,2}$`)
)

// arith rejects everything that isn't plain arithmetic, and records whether
// the expression actually computes something (so "2024" alone isn't an answer).
type arith struct {
	bad, op bool
}

func (v *arith) Visit(n *ast.Node) {
	switch n := (*n).(type) {
	case *ast.IntegerNode, *ast.FloatNode:
	case *ast.IdentifierNode:
		if _, ok := mathEnv[n.Value]; !ok {
			v.bad = true
		}
	case *ast.UnaryNode:
		if n.Operator != "-" && n.Operator != "+" {
			v.bad = true
		}
	case *ast.BinaryNode:
		switch n.Operator {
		case "+", "-", "*", "/", "%", "^", "**":
			v.op = true
		default:
			v.bad = true
		}
	case *ast.CallNode:
		if _, ok := n.Callee.(*ast.IdentifierNode); !ok {
			v.bad = true
		}
		v.op = true
	default:
		v.bad = true
	}
}

// floats makes all arithmetic float64, so big products don't wrap around
// and % works on decimals.
type floats struct{}

func (floats) Visit(n *ast.Node) {
	switch m := (*n).(type) {
	case *ast.IntegerNode:
		ast.Patch(n, &ast.FloatNode{Value: float64(m.Value)})
	case *ast.BinaryNode:
		if m.Operator == "%" {
			ast.Patch(n, &ast.CallNode{Callee: &ast.IdentifierNode{Value: "mod"}, Arguments: []ast.Node{m.Left, m.Right}})
		}
	}
}

func calc(q string) *Answer {
	s := strings.TrimSpace(strings.TrimRight(calcPrefix.ReplaceAllString(q, ""), "=? "))
	if isoDate.MatchString(s) {
		return nil
	}
	s = strings.NewReplacer("×", "*", "÷", "/", "−", "-", "π", "pi", "√", "sqrt").Replace(s)
	s = timesX.ReplaceAllString(s, "$1*$2")
	tree, err := parser.Parse(s)
	if err != nil {
		return nil
	}
	var chk arith
	ast.Walk(&tree.Node, &chk)
	if chk.bad || !chk.op {
		return nil
	}
	shown := tree.Node.String()
	prog, err := expr.Compile(s, expr.Env(mathEnv), expr.DisableAllBuiltins(), expr.Patch(floats{}), expr.AsFloat64())
	if err != nil {
		return nil
	}
	out, err := expr.Run(prog, mathEnv)
	if err != nil {
		return nil
	}
	f, ok := out.(float64)
	if !ok {
		return nil
	}
	res := formatNum(f)
	if res == "" {
		return nil
	}
	return &Answer{Kind: "calc", Expr: shown, Result: res}
}
