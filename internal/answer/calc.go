package answer

import (
	"maps"
	"math"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
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
// Identifiers other than mathEnv's are collected in free (variables of an integrand).
type arith struct {
	bad, op bool
	free    map[string]bool
}

func (v *arith) Visit(n *ast.Node) {
	switch n := (*n).(type) {
	case *ast.IntegerNode, *ast.FloatNode:
	case *ast.IdentifierNode:
		if _, ok := mathEnv[n.Value]; !ok {
			if v.free == nil {
				v.bad = true
			} else {
				v.free[n.Value] = true
			}
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

var symbols = strings.NewReplacer("×", "*", "÷", "/", "−", "-", "π", "pi", "√", "sqrt")

// parseArith parses s and checks that it is plain arithmetic. With free non-nil,
// unknown identifiers are allowed and recorded there instead of rejected.
func parseArith(s string, free map[string]bool) (tree *parser.Tree, op bool, ok bool) {
	tree, err := parser.Parse(s)
	if err != nil {
		return nil, false, false
	}
	chk := arith{free: free}
	ast.Walk(&tree.Node, &chk)
	return tree, chk.op, !chk.bad
}

// compileArith compiles an expression already checked by parseArith. vars are
// extra float64 variables (e.g. an integrand's x), set in the env passed to expr.Run.
func compileArith(s string, vars ...string) (*vm.Program, error) {
	env := mathEnv
	if len(vars) > 0 {
		env = maps.Clone(mathEnv)
		for _, v := range vars {
			env[v] = 0.0
		}
	}
	return expr.Compile(s, expr.Env(env), expr.DisableAllBuiltins(), expr.Patch(floats{}), expr.AsFloat64())
}

func runFloat(prog *vm.Program, env map[string]any) (float64, bool) {
	out, err := expr.Run(prog, env)
	if err != nil {
		return 0, false
	}
	f, ok := out.(float64)
	return f, ok
}

func calc(q string) *Answer {
	s := strings.TrimSpace(strings.TrimRight(calcPrefix.ReplaceAllString(q, ""), "=? "))
	if isoDate.MatchString(s) {
		return nil
	}
	s = symbols.Replace(s)
	s = timesX.ReplaceAllString(s, "$1*$2")
	tree, op, ok := parseArith(s, nil)
	if !ok || !op {
		return nil
	}
	prog, err := compileArith(s)
	if err != nil {
		return nil
	}
	f, ok := runFloat(prog, mathEnv)
	if !ok {
		return nil
	}
	shown := tree.Node.String()
	res := formatNum(f)
	if res == "" {
		return nil
	}
	return &Answer{Kind: "calc", Expr: shown, Result: res}
}
