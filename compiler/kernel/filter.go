package kernel

import (
	"fmt"

	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/expr"
	"github.com/brimdata/zed/zio/tzngio"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

func compileCompareField(zctx *zson.Context, scope *Scope, e *dag.BinaryExpr) (expr.Filter, error) {
	if e.Op == "in" {
		literal, ok := e.LHS.(*zed.Primitive)
		if !ok {
			// XXX If the RHS here is a literal container or a subnet,
			// we should optimize this case.  This is part of
			// epic #2341.
			return nil, nil
		}
		// Check if RHS is a legit lval/field.
		if _, err := compileLval(e.RHS); err != nil {
			return nil, nil
		}
		resolver, err := compileExpr(zctx, scope, e.RHS)
		if err != nil {
			return nil, err
		}
		eql, _ := expr.Comparison("=", *literal)
		comparison := expr.Contains(eql)
		return expr.Apply(resolver, comparison), nil
	}
	literal, ok := e.RHS.(*zed.Primitive)
	if !ok {
		return nil, nil
	}
	comparison, err := expr.Comparison(e.Op, *literal)
	if err != nil {
		// If this fails, return no match instead of the error and
		// let later-on code detect the error as this could be a
		// non-error situation that isn't a simple comparison.
		return nil, nil
	}
	// Check if LHS is a legit lval/field before compiling the expr.
	if _, err := compileLval(e.LHS); err != nil {
		return nil, nil
	}
	resolver, err := compileExpr(zctx, scope, e.LHS)
	if err != nil {
		return nil, err
	}
	return expr.Apply(resolver, comparison), nil
}

func compileSearch(node *dag.Search) (expr.Filter, error) {
	if node.Value.Type == "net" {
		match, err := expr.Comparison("=", node.Value)
		if err != nil {
			return nil, err
		}
		contains := expr.Contains(match)
		pred := func(zv zng.Value) bool {
			return match(zv) || contains(zv)
		}

		return expr.EvalAny(pred, true), nil
	}

	if node.Value.Type == "string" {
		term, err := tzngio.ParseBstring([]byte(node.Value.Text))
		if err != nil {
			return nil, err
		}
		return expr.SearchRecordString(string(term)), nil
	}

	return expr.SearchRecordOther(node.Text, node.Value)
}

func CompileFilter(zctx *zson.Context, scope *Scope, node dag.Expr) (expr.Filter, error) {
	switch v := node.(type) {
	case *dag.RegexpMatch:
		e, err := compileExpr(zctx, scope, v.Expr)
		if err != nil {
			return nil, err
		}
		re, err := expr.CompileRegexp(v.Pattern)
		if err != nil {
			return nil, err
		}
		pred := expr.NewRegexpBoolean(re)
		return expr.Apply(e, pred), nil

	case *dag.RegexpSearch:
		re, err := expr.CompileRegexp(v.Pattern)
		if err != nil {
			return nil, err
		}
		match := expr.NewRegexpBoolean(re)
		contains := expr.Contains(match)
		pred := func(zv zng.Value) bool {
			return match(zv) || contains(zv)
		}
		return expr.EvalAny(pred, true), nil

	case *dag.UnaryExpr:
		if v.Op != "!" {
			return nil, fmt.Errorf("unknown unary operator: %s", v.Op)
		}
		e, err := CompileFilter(zctx, scope, v.Operand)
		if err != nil {
			return nil, err
		}
		return expr.LogicalNot(e), nil

	case *zed.Primitive:
		// This literal translation should happen elsewhere will
		// be fixed when we add ZSON literals to Z, e.g.,
		// dag.Literal.AsBool() etc methods.
		if v.Type != "bool" {
			return nil, fmt.Errorf("bad literal type in filter compiler: %s", v.Type)
		}
		var b bool
		switch v.Text {
		case "true":
			b = true
		case "false":
		default:
			return nil, fmt.Errorf("bad boolean value in dag.Literal: %s", v.Text)
		}
		return func(*zng.Record) bool { return b }, nil

	case *dag.Search:
		return compileSearch(v)

	case *dag.BinaryExpr:
		if v.Op == "and" {
			left, err := CompileFilter(zctx, scope, v.LHS)
			if err != nil {
				return nil, err
			}
			right, err := CompileFilter(zctx, scope, v.RHS)
			if err != nil {
				return nil, err
			}
			return expr.LogicalAnd(left, right), nil
		}
		if v.Op == "or" {
			left, err := CompileFilter(zctx, scope, v.LHS)
			if err != nil {
				return nil, err
			}
			right, err := CompileFilter(zctx, scope, v.RHS)
			if err != nil {
				return nil, err
			}
			return expr.LogicalOr(left, right), nil
		}
		if f, err := compileCompareField(zctx, scope, v); f != nil || err != nil {
			return f, err
		}
		return compileExprPredicate(zctx, scope, v)

	case *dag.Call:
		return compileExprPredicate(zctx, scope, v)
	case *dag.SeqExpr:
		if f, err := compileCompareAny(v); f != nil || err != nil {
			return f, err
		}
		return compileExprPredicate(zctx, scope, v)

	default:
		return nil, fmt.Errorf("Filter AST unknown type: %v", v)
	}
}

func compileExprPredicate(zctx *zson.Context, scope *Scope, e dag.Expr) (expr.Filter, error) {
	predicate, err := compileExpr(zctx, scope, e)
	if err != nil {
		return nil, err
	}
	return func(rec *zng.Record) bool {
		zv, err := predicate.Eval(rec)
		if err != nil {
			return false
		}
		if zv.Type == zng.TypeBool && zng.IsTrue(zv.Bytes) {
			return true
		}
		return false
	}, nil
}

func compileCompareAny(e *dag.SeqExpr) (expr.Filter, error) {
	literal, op, ok := isCompareAny(e)
	if !ok {
		return nil, nil
	}
	var pred expr.Boolean
	var err error
	if op == "in" {
		comparison, err := expr.Comparison("=", *literal)
		if err != nil {
			return nil, err
		}
		pred = expr.Contains(comparison)
	} else {
		pred, err = expr.Comparison(op, *literal)
		if err != nil {
			return nil, err
		}
	}
	return expr.EvalAny(pred, false), nil
}
