package kernel

import (
	"errors"
	"fmt"

	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/expr"
	"github.com/brimdata/zed/expr/agg"
	"github.com/brimdata/zed/expr/function"
	"github.com/brimdata/zed/field"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

// compileExpr compiles the given Expression into an object
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
func compileExpr(zctx *zson.Context, scope *Scope, e dag.Expr) (expr.Evaluator, error) {
	if e == nil {
		return nil, errors.New("null expression not allowed")
	}
	switch e := e.(type) {
	case *zed.Primitive:
		zv, err := zson.ParsePrimitive(e.Type, e.Text)
		if err != nil {
			return nil, err
		}
		return expr.NewLiteral(zv), nil
	case *dag.Ref:
		// If the reference refers to a named variable in scope (like "$"),
		// then return a Var expression referring to the pointer to the value.
		// Note that constants may be accessed this way too by entering their
		// names into the global (outermost) scope in the Scope entity.
		if ref := scope.Lookup(e.Name); ref != nil {
			return expr.NewVar(ref), nil
		}
		return nil, fmt.Errorf("unknown reference: '%s'", e.Name)
	case *dag.Search:
		f, err := compileSearch(e)
		if err != nil {
			return nil, err
		}
		return expr.FilterEvaluator(f), nil
	case *dag.Path:
		return expr.NewDotExpr(field.Static(e.Name)), nil
	case *dag.Dot:
		return compileDotExpr(zctx, scope, e)
	case *dag.UnaryExpr:
		return compileUnary(zctx, scope, *e)
	case *dag.SelectExpr:
		return nil, errors.New("Z kernel: encountered select expression")
	case *dag.BinaryExpr:
		return compileBinary(zctx, scope, e)
	case *dag.Conditional:
		return compileConditional(zctx, scope, *e)
	case *dag.Call:
		return compileCall(zctx, scope, *e)
	case *dag.Cast:
		return compileCast(zctx, scope, *e)
	case *zed.TypeValue:
		return compileTypeValue(zctx, scope, e)
	case *dag.SeqExpr:
		return compileSeqExpr(zctx, scope, e)
	case *dag.RegexpMatch:
		return compileRegexpMatch(zctx, scope, e)
	case *dag.RecordExpr:
		return compileRecordExpr(zctx, scope, e)
	case *dag.ArrayExpr:
		return compileArrayExpr(zctx, scope, e)
	case *dag.SetExpr:
		return compileSetExpr(zctx, scope, e)
	case *dag.MapExpr:
		return compileMapExpr(zctx, scope, e)
	default:
		return nil, fmt.Errorf("invalid expression type %T", e)
	}
}

func compileExprWithEmpty(zctx *zson.Context, scope *Scope, e dag.Expr) (expr.Evaluator, error) {
	if e == nil {
		return nil, nil
	}
	return compileExpr(zctx, scope, e)
}

func CompileExprs(zctx *zson.Context, scope *Scope, nodes []dag.Expr) ([]expr.Evaluator, error) {
	var exprs []expr.Evaluator
	for k := range nodes {
		e, err := compileExpr(zctx, scope, nodes[k])
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
	}
	return exprs, nil
}

func compileBinary(zctx *zson.Context, scope *Scope, e *dag.BinaryExpr) (expr.Evaluator, error) {
	if slice, ok := e.RHS.(*dag.BinaryExpr); ok && slice.Op == ":" {
		return compileSlice(zctx, scope, e.LHS, slice)
	}
	lhs, err := compileExpr(zctx, scope, e.LHS)
	if err != nil {
		return nil, err
	}
	rhs, err := compileExpr(zctx, scope, e.RHS)
	if err != nil {
		return nil, err
	}
	switch op := e.Op; op {
	case "and", "or":
		return compileLogical(lhs, rhs, op)
	case "in":
		return expr.NewIn(lhs, rhs), nil
	case "=", "!=":
		return expr.NewCompareEquality(lhs, rhs, op)
	case "<", "<=", ">", ">=":
		return expr.NewCompareRelative(lhs, rhs, op)
	case "+", "-", "*", "/":
		return expr.NewArithmetic(lhs, rhs, op)
	case "[":
		return expr.NewIndexExpr(zctx, lhs, rhs)
	default:
		return nil, fmt.Errorf("Z kernel: invalid binary operator %s", op)
	}
}

func compileSlice(zctx *zson.Context, scope *Scope, container dag.Expr, slice *dag.BinaryExpr) (expr.Evaluator, error) {
	from, err := compileExprWithEmpty(zctx, scope, slice.LHS)
	if err != nil {
		return nil, err
	}
	to, err := compileExprWithEmpty(zctx, scope, slice.RHS)
	if err != nil {
		return nil, err
	}
	e, err := compileExpr(zctx, scope, container)
	if err != nil {
		return nil, err
	}
	return expr.NewSlice(e, from, to), nil
}

func compileSeqExpr(zctx *zson.Context, scope *Scope, seq *dag.SeqExpr) (expr.Evaluator, error) {
	selectors, err := compileExprs(zctx, scope, seq.Selectors)
	if err != nil {
		return nil, err
	}
	selector := expr.NewSelector(selectors)
	sequence := expr.Generator(selector)
	for _, method := range seq.Methods {
		sequence, err = compileMethod(zctx, scope, sequence, method)
		if err != nil {
			return nil, err
		}
	}
	pattern, err := agg.NewPattern(seq.Name)
	if err != nil {
		return nil, err
	}
	return expr.NewAggExpr(zctx, pattern, sequence), nil
}

func compileMethod(zctx *zson.Context, scope *Scope, src expr.Generator, method dag.Method) (expr.Generator, error) {
	switch method.Name {
	case "map":
		if len(method.Args) != 1 {
			return nil, errors.New("map() method requires one argument")
		}
		mapMethod := expr.NewMapMethod(src)
		scope.Enter()
		defer scope.Exit()
		scope.Bind("$", mapMethod.Ref())
		mapExpr, err := compileExpr(zctx, scope, method.Args[0])
		if err != nil {
			return nil, err
		}
		mapMethod.Set(mapExpr)
		return mapMethod, nil
	case "filter":
		if len(method.Args) != 1 {
			return nil, errors.New("filter() method requires one argument")
		}
		filterMethod := expr.NewFilterMethod(src)
		scope.Enter()
		defer scope.Exit()
		scope.Bind("$", filterMethod.Ref())
		filterExpr, err := compileExpr(zctx, scope, method.Args[0])
		if err != nil {
			fmt.Println("ERR", err)
			return nil, err
		}
		filterMethod.Set(filterExpr)
		return filterMethod, nil
	default:
		return nil, fmt.Errorf("uknown method: %s", method.Name)
	}
}

func compileUnary(zctx *zson.Context, scope *Scope, unary dag.UnaryExpr) (expr.Evaluator, error) {
	if unary.Op != "!" {
		return nil, fmt.Errorf("unknown unary operator %s\n", unary.Op)
	}
	e, err := compileExpr(zctx, scope, unary.Operand)
	if err != nil {
		return nil, err
	}
	return expr.NewLogicalNot(e), nil
}

func compileLogical(lhs, rhs expr.Evaluator, operator string) (expr.Evaluator, error) {
	switch operator {
	case "and":
		return expr.NewLogicalAnd(lhs, rhs), nil
	case "or":
		return expr.NewLogicalOr(lhs, rhs), nil
	default:
		return nil, fmt.Errorf("unknown logical operator: %s", operator)
	}
}

func compileConditional(zctx *zson.Context, scope *Scope, node dag.Conditional) (expr.Evaluator, error) {
	predicate, err := compileExpr(zctx, scope, node.Cond)
	if err != nil {
		return nil, err
	}
	thenExpr, err := compileExpr(zctx, scope, node.Then)
	if err != nil {
		return nil, err
	}
	elseExpr, err := compileExpr(zctx, scope, node.Else)
	if err != nil {
		return nil, err
	}
	return expr.NewConditional(predicate, thenExpr, elseExpr), nil
}

func compileDotExpr(zctx *zson.Context, scope *Scope, dot *dag.Dot) (expr.Evaluator, error) {
	record, err := compileExpr(zctx, scope, dot.LHS)
	if err != nil {
		return nil, err
	}
	return expr.NewDotAccess(record, dot.RHS), nil
}

func compileCast(zctx *zson.Context, scope *Scope, node dag.Cast) (expr.Evaluator, error) {
	e, err := compileExpr(zctx, scope, node.Expr)
	if err != nil {
		return nil, err
	}
	//XXX we should handle runtime resolution of typedef names
	typ, err := zson.TranslateType(zctx, node.Type)
	if err != nil {
		return nil, err
	}
	return expr.NewCast(e, typ)
}

func compileLval(e dag.Expr) (field.Static, error) {
	if e, ok := e.(*dag.Path); ok {
		return field.Static(e.Name), nil
	}
	return nil, errors.New("invalid expression on lhs of assignment")
}

func CompileAssignment(zctx *zson.Context, scope *Scope, node *dag.Assignment) (expr.Assignment, error) {
	lhs, err := compileLval(node.LHS)
	if err != nil {
		return expr.Assignment{}, err
	}
	rhs, err := compileExpr(zctx, scope, node.RHS)
	if err != nil {
		return expr.Assignment{}, fmt.Errorf("rhs of assigment expression: %w", err)
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

func compileCutter(zctx *zson.Context, scope *Scope, node dag.Call) (*expr.Cutter, error) {
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
		assignment := &dag.Assignment{LHS: expr, RHS: expr}
		compiled, err := CompileAssignment(zctx, scope, assignment)
		if err != nil {
			return nil, err
		}
		lhs = append(lhs, compiled.LHS)
		rhs = append(rhs, compiled.RHS)
	}
	return expr.NewCutter(zctx, lhs, rhs)
}

func shaperOps(name string) expr.ShaperTransform {
	switch name {
	case "cast":
		return expr.Cast
	case "crop":
		return expr.Crop
	case "fill":
		return expr.Fill
	case "fit":
		return expr.Crop | expr.Fill
	case "order":
		return expr.Order
	case "shape":
		return expr.Cast | expr.Fill | expr.Order
	default:
		return 0
	}

}

func isShaperFunc(name string) bool {
	return shaperOps(name) != 0
}

func compileShaper(zctx *zson.Context, scope *Scope, node dag.Call) (*expr.Shaper, error) {
	args := node.Args
	if len(args) == 1 {
		args = append([]dag.Expr{dag.Root}, args...)
	}
	if len(args) < 2 {
		return nil, function.ErrTooFewArgs
	}
	if len(args) > 2 {
		return nil, function.ErrTooManyArgs
	}
	field, err := compileExpr(zctx, scope, args[0])
	if err != nil {
		return nil, err
	}
	typExpr, err := compileExpr(zctx, scope, args[1])
	if err != nil {
		return nil, err
	}
	// XXX When we do constant folding, we should detect when typeExpr is
	// a constant and allocate a ConstShaper instead of a (dynamic) Shaper.
	// See issue #2425.
	return expr.NewShaper(zctx, field, typExpr, shaperOps(node.Name)), nil
}

func compileCall(zctx *zson.Context, scope *Scope, call dag.Call) (expr.Evaluator, error) {
	// For now, we special case stateful functions here.  We should generalize this
	// as we will add many more stateful functions and also resolve this
	// the changes to create running aggegation functions from reducers.
	// XXX See issue #1259.
	switch {
	case call.Name == "cut":
		cut, err := compileCutter(zctx, scope, call)
		if err != nil {
			return nil, err
		}
		cut.AllowPartialCuts()
		return cut, nil
	case call.Name == "pick":
		return compileCutter(zctx, scope, call)
	case call.Name == "exists":
		exprs, err := compileExprs(zctx, scope, call.Args)
		if err != nil {
			return nil, fmt.Errorf("exists(): bad argument: %w", err)
		}
		return expr.NewExists(zctx, exprs), nil
	case call.Name == "missing":
		exprs, err := compileExprs(zctx, scope, call.Args)
		if err != nil {
			return nil, fmt.Errorf("missing(): bad argument: %w", err)
		}
		return expr.NewMissing(exprs), nil
	case call.Name == "has":
		exprs, err := compileExprs(zctx, scope, call.Args)
		if err != nil {
			return nil, fmt.Errorf("has(): bad argument: %w", err)
		}
		return expr.NewHas(exprs), nil
	case call.Name == "unflatten":
		return expr.NewUnflattener(zctx), nil
	case isShaperFunc(call.Name):
		return compileShaper(zctx, scope, call)
	}
	nargs := len(call.Args)
	fn, root, err := function.New(zctx, call.Name, nargs)
	if err != nil {
		return nil, fmt.Errorf("%s(): %w", call.Name, err)
	}
	args := call.Args
	if root {
		args = append([]dag.Expr{dag.Root}, args...)
	}
	exprs, err := compileExprs(zctx, scope, args)
	if err != nil {
		return nil, fmt.Errorf("%s(): bad argument: %w", call.Name, err)
	}
	return expr.NewCall(zctx, fn, exprs), nil
}

func compileExprs(zctx *zson.Context, scope *Scope, in []dag.Expr) ([]expr.Evaluator, error) {
	out := make([]expr.Evaluator, 0, len(in))
	for _, e := range in {
		ev, err := compileExpr(zctx, scope, e)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}

func compileTypeValue(zctx *zson.Context, scope *Scope, t *zed.TypeValue) (expr.Evaluator, error) {
	if typ, ok := t.Value.(*zed.TypeName); ok {
		// We currently support dynamic type names only for
		// top-level type names.  By dynamic, we mean typedefs that
		// come from the data instead of the Z.  For dynamic type
		// names that are embedded lower down in a complex type,
		// we need to implement some type of tracker objec that
		// can resolve the type when all the dependent types are found.
		// See issue #2182.
		return expr.NewTypeFunc(zctx, typ.Name), nil
	}
	typ, err := zson.TranslateType(zctx, t.Value)
	if err != nil {
		return nil, err
	}
	return expr.NewLiteral(zng.NewTypeType(typ)), nil
}

func compileRegexpMatch(zctx *zson.Context, scope *Scope, match *dag.RegexpMatch) (expr.Evaluator, error) {
	e, err := compileExpr(zctx, scope, match.Expr)
	if err != nil {
		return nil, err
	}
	re, err := expr.CompileRegexp(match.Pattern)
	if err != nil {
		return nil, err
	}
	return expr.NewRegexpMatch(re, e), nil
}

func compileRegexpSearch(zctx *zson.Context, scope *Scope, search *dag.RegexpSearch) (expr.Evaluator, error) {
	re, err := expr.CompileRegexp(search.Pattern)
	if err != nil {
		return nil, err
	}
	return expr.NewRegexpSearch(re), nil
}

func compileRecordExpr(zctx *zson.Context, scope *Scope, record *dag.RecordExpr) (expr.Evaluator, error) {
	var names []string
	var exprs []expr.Evaluator
	for _, f := range record.Fields {
		e, err := compileExpr(zctx, scope, f.Value)
		if err != nil {
			return nil, err
		}
		names = append(names, f.Name)
		exprs = append(exprs, e)
	}
	return expr.NewRecordExpr(zctx, names, exprs), nil
}

func compileArrayExpr(zctx *zson.Context, scope *Scope, array *dag.ArrayExpr) (expr.Evaluator, error) {
	exprs, err := compileExprs(zctx, scope, array.Exprs)
	if err != nil {
		return nil, err
	}
	return expr.NewArrayExpr(zctx, exprs), nil
}

func compileSetExpr(zctx *zson.Context, scope *Scope, set *dag.SetExpr) (expr.Evaluator, error) {
	exprs, err := compileExprs(zctx, scope, set.Exprs)
	if err != nil {
		return nil, err
	}
	return expr.NewSetExpr(zctx, exprs), nil
}

func compileMapExpr(zctx *zson.Context, scope *Scope, m *dag.MapExpr) (expr.Evaluator, error) {
	var entries []expr.Entry
	for _, f := range m.Entries {
		key, err := compileExpr(zctx, scope, f.Key)
		if err != nil {
			return nil, err
		}
		val, err := compileExpr(zctx, scope, f.Value)
		if err != nil {
			return nil, err
		}
		entries = append(entries, expr.Entry{key, val})
	}
	return expr.NewMapExpr(zctx, entries), nil
}
