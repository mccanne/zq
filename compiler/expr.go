package compiler

import (
	"errors"
	"fmt"

	"github.com/brimsec/zq/ast"
	"github.com/brimsec/zq/expr"
	"github.com/brimsec/zq/expr/function"
	"github.com/brimsec/zq/field"
	"github.com/brimsec/zq/zng/resolver"
)

// CompileExpr compiles the given Expression into an object
// that evaluates the expression against a provided Record.  It returns an
// error if compilation fails for any reason.
//
// This is the "intepreted slow path" of the analytics engine.  Because it
// handles dynamic typing at runtime, overheads are incurred due to
// various type checks and coercions that determine different computational
// outcomes based on type.  There is nothing here that optimizes analytics
// for native machine types; these optimizations (will) happen in the pushdown
// predicate processing engine in the zst columnar scanner.
//
// Eventually, we will optimize this zst "fast path" by dynamically
// generating byte codes (which can in turn be JIT assembled into machine code)
// for each zng TypeRecord encountered.  Once you know the TypeRecord,
// you can generate code using strong typing just as an OLAP system does
// due to its schemas defined up-front in its relational tables.  Here,
// each record type is like a schema and as we encounter them, we can compile
// optimized code for the now-static types within that record type.
//
// The Evaluator return by CompileExpr produces zng.Values that are stored
// in temporary buffers and may be modified on subsequent calls to Eval.
// This is intended to minimize the garbage collection needs of the inner loop
// by not allocating memory on a per-Eval basis.  For uses like filtering and
// aggregations, where the results are immediately used, this is desirable and
// efficient but for use cases like storing the results as groupby keys, the
// resulting zng.Value should be copied (e.g., via zng.Value.Copy()).
//
// TBD: string values and net.IP address do not need to be copied because they
// are allocated by go libraries and temporary buffers are not used.  This will
// change down the road when we implement no-allocation string and IP conversion.
func CompileExpr(zctx *resolver.Context, node ast.Expression) (expr.Evaluator, error) {
	switch n := node.(type) {
	case *ast.Literal:
		return expr.NewLiteral(*n)
	case *ast.Identifier:
		return nil, fmt.Errorf("stray identifier in AST: %s", n.Name)
	case *ast.RootRecord:
		return &expr.RootRecord{}, nil
	case *ast.UnaryExpression:
		return compileUnary(zctx, *n)
	case *ast.BinaryExpression:
		return compileBinary(zctx, n.Operator, n.LHS, n.RHS)
	case *ast.ConditionalExpression:
		return compileConditional(zctx, *n)
	case *ast.FunctionCall:
		return compileCall(zctx, *n)
	case *ast.CastExpression:
		return compileCast(zctx, *n)
	default:
		return nil, fmt.Errorf("invalid expression type %T", node)
	}
}

func CompileExprs(zctx *resolver.Context, nodes []ast.Expression) ([]expr.Evaluator, error) {
	var exprs []expr.Evaluator
	for k := range nodes {
		e, err := CompileExpr(zctx, nodes[k])
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
	}
	return exprs, nil
}

func compileBinary(zctx *resolver.Context, op string, LHS, RHS ast.Expression) (expr.Evaluator, error) {
	if op == "." {
		return compileDotExpr(zctx, LHS, RHS)
	}
	lhs, err := CompileExpr(zctx, LHS)
	if err != nil {
		return nil, err
	}
	rhs, err := CompileExpr(zctx, RHS)
	if err != nil {
		return nil, err
	}
	switch op {
	case "AND", "OR":
		return compileLogical(lhs, rhs, op)
	case "in":
		return expr.NewIn(lhs, rhs), nil
	case "=", "!=":
		return expr.NewCompareEquality(lhs, rhs, op)
	case "=~", "!~":
		return expr.NewPatternMatch(lhs, rhs, op)
	case "<", "<=", ">", ">=":
		return expr.NewCompareRelative(lhs, rhs, op)
	case "+", "-", "*", "/":
		return expr.NewArithmetic(lhs, rhs, op)
	case "[":
		return expr.NewIndexExpr(zctx, lhs, rhs)
	default:
		return nil, fmt.Errorf("invalid binary operator %s", op)
	}
}

func compileUnary(zctx *resolver.Context, node ast.UnaryExpression) (expr.Evaluator, error) {
	if node.Operator != "!" {
		return nil, fmt.Errorf("unknown unary operator %s\n", node.Operator)
	}
	e, err := CompileExpr(zctx, node.Operand)
	if err != nil {
		return nil, err
	}
	return expr.NewLogicalNot(e), nil
}

func compileLogical(lhs, rhs expr.Evaluator, operator string) (expr.Evaluator, error) {
	switch operator {
	case "AND":
		return expr.NewLogicalAnd(lhs, rhs), nil
	case "OR":
		return expr.NewLogicalOr(lhs, rhs), nil
	default:
		return nil, fmt.Errorf("unknown logical operator: %s", operator)
	}
}

func compileConditional(zctx *resolver.Context, node ast.ConditionalExpression) (expr.Evaluator, error) {
	predicate, err := CompileExpr(zctx, node.Condition)
	if err != nil {
		return nil, err
	}
	thenExpr, err := CompileExpr(zctx, node.Then)
	if err != nil {
		return nil, err
	}
	elseExpr, err := CompileExpr(zctx, node.Else)
	if err != nil {
		return nil, err
	}
	return expr.NewConditional(predicate, thenExpr, elseExpr), nil
}

func compileDotExpr(zctx *resolver.Context, lhs, rhs ast.Expression) (expr.Evaluator, error) {
	id, ok := rhs.(*ast.Identifier)
	if !ok {
		return nil, errors.New("rhs of dot expression is not an identifier")
	}
	record, err := CompileExpr(zctx, lhs)
	if err != nil {
		return nil, err
	}
	return expr.NewDotAccess(record, id.Name), nil
}

func compileCast(zctx *resolver.Context, node ast.CastExpression) (expr.Evaluator, error) {
	e, err := CompileExpr(zctx, node.Expr)
	if err != nil {
		return nil, err
	}
	return expr.NewCast(e, node.Type)
}

func CompileLval(node ast.Expression) (field.Static, error) {
	switch node := node.(type) {
	case *ast.RootRecord:
		return field.NewRoot(), nil
	// XXX We need to allow index operators at some point, but for now
	// we have been assuming only dotted field lvalues.  See Issue #1462.
	case *ast.BinaryExpression:
		if node.Operator != "." {
			break
		}
		id, ok := node.RHS.(*ast.Identifier)
		if !ok {
			return nil, errors.New("rhs of dot operator is not an identifier")
		}
		lhs, err := CompileLval(node.LHS)
		if err != nil {
			return nil, err
		}
		return append(lhs, id.Name), nil
	}
	return nil, errors.New("invalid expression on lhs of assignment")
}

func CompileAssignment(zctx *resolver.Context, node *ast.Assignment) (expr.Assignment, error) {
	rhs, err := CompileExpr(zctx, node.RHS)
	if err != nil {
		return expr.Assignment{}, fmt.Errorf("rhs of assigment expression: %w", err)
	}
	var lhs field.Static
	if node.LHS != nil {
		lhs, err = CompileLval(node.LHS)
		if err != nil {
			return expr.Assignment{}, fmt.Errorf("lhs of assigment expression: %w", err)
		}
	} else {
		switch rhs := node.RHS.(type) {
		case *ast.RootRecord:
			lhs = field.New(".")
		case *ast.FunctionCall:
			lhs = field.New(rhs.Function)
		case *ast.BinaryExpression:
			// This can be a dotted record or some other expression.
			// In the latter case, it might be nice to infer a name,
			// e.g., forr "count() by a+b" we could infer "sum" for
			// the name, i,e., "count() by sum=a+b".  But for now,
			// we'll just catch this as an error.
			lhs, err = CompileLval(rhs)
			if err != nil {
				err = expr.ErrInference
			}
		default:
			err = expr.ErrInference
		}
	}
	return expr.Assignment{lhs, rhs}, err
}

func CompileAssignments(dsts []field.Static, srcs []field.Static) ([]field.Static, []expr.Evaluator) {
	if len(srcs) != len(dsts) {
		panic("CompileAssignments: argument mismatch")
	}
	var resolvers []expr.Evaluator
	var fields []field.Static
	for k, dst := range dsts {
		fields = append(fields, dst)
		resolvers = append(resolvers, expr.NewDotExpr(srcs[k]))
	}
	return fields, resolvers
}

func compileCutter(zctx *resolver.Context, node ast.FunctionCall) (*expr.Cutter, error) {
	var lhs []field.Static
	var rhs []expr.Evaluator
	for _, expr := range node.Args {
		// This is a bit of a hack and could be cleaed up by re-factoring
		// CompileAssigment, but for now, we create an assigment expression
		// where the LHS and RHS are the same, so that cut(id.orig_h,_path)
		// gives a value of type {id:{orig_h:ip},{_path:string}}
		// with field names that are the same as the cut names.
		// We should allow field assignments as function arguments.
		// See issue #1772.
		assignment := &ast.Assignment{LHS: expr, RHS: expr}
		compiled, err := CompileAssignment(zctx, assignment)
		if err != nil {
			return nil, err
		}
		lhs = append(lhs, compiled.LHS)
		rhs = append(rhs, compiled.RHS)
	}
	return expr.NewCutter(zctx, lhs, rhs)
}

func compileCall(zctx *resolver.Context, node ast.FunctionCall) (expr.Evaluator, error) {
	// For now, we special case cut and pick here.  We shuold generalize this
	// as we will add many more stateful functions and also resolve this
	// the changes to create running aggegation functions from reducers.
	// XXX See issue #1259.
	if node.Function == "cut" {
		cut, err := compileCutter(zctx, node)
		if err != nil {
			return nil, err
		}
		cut.AllowPartialCuts()
		return cut, nil
	}
	if node.Function == "pick" {
		return compileCutter(zctx, node)
	}
	nargs := len(node.Args)
	fn, err := function.New(zctx, node.Function, nargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", node.Function, err)
	}
	exprs := make([]expr.Evaluator, 0, nargs)
	for _, expr := range node.Args {
		e, err := CompileExpr(zctx, expr)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
	}
	return expr.NewCall(zctx, node.Function, fn, exprs), nil
}
