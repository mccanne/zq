package semantic

import (
	"errors"
	"fmt"

	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/expr/agg"
	"github.com/brimdata/zed/field"
)

func convertSQLProc(scope *Scope, sql *ast.SQLExpr) (dag.Op, error) {
	selection, err := newSQLSelection(scope, sql.Select)
	if err != err {
		return nil, err
	}
	var where dag.Expr
	if sql.Where != nil {
		where, err = semExpr(scope, sql.Where)
		if err != nil {
			return nil, err
		}
	}
	var ops []dag.Op
	if sql.From != nil {
		alias, aliasID, err := convertSQLAlias(scope, sql.From.Alias)
		if err != nil {
			return nil, err
		}
		tableFilter, err := convertSQLTableRef(scope, sql.From.Table)
		if err != nil {
			return nil, err
		}
		ops = append(ops, tableFilter)
		if aliasID != "" {
			// If the FROM table has been aliased and all join clauses, if any,
			// are also aliased, then we can lift any where claused that is dependent
			// only on the FROM table or is a component of a logical AND and that
			// component depends only on the FROM table.  In this case, we splice
			// the filter predicate after the FROM expression and before everything else.
			// This is a "peephole" optimization that will go away once we
			// have fully fledge data-flow-based optimizations.
			if where != nil {
				if f := liftWhereFilter(aliasID, where, sql.Joins); f != nil {
					ops = append(ops, f)
				}
			}
			ops = append(ops, alias)
		}
	}
	if sql.Joins != nil {
		if len(ops) == 0 {
			return nil, errors.New("cannot JOIN without a FROM")
		}
		ops, err = convertSQLJoins(scope, ops, sql.Joins)
		if err != nil {
			return nil, err
		}
	}
	if where != nil {
		filter := &dag.Filter{
			Kind: "Filter",
			Expr: where,
		}
		ops = append(ops, filter)
	}
	if sql.GroupBy != nil {
		groupby, err := convertSQLGroupBy(scope, sql.GroupBy, selection)
		if err != nil {
			return nil, err
		}
		ops = append(ops, groupby)
		if sql.Having != nil {
			having, err := semExpr(scope, sql.Having)
			if err != nil {
				return nil, err
			}
			filter := &dag.Filter{
				Kind: "Filter",
				Expr: having,
			}
			ops = append(ops, filter)
		}
	} else if sql.Select != nil {
		if sql.Having != nil {
			return nil, errors.New("HAVING clause used without GROUP BY")
		}
		// GroupBy will do the cutting but if there's no GroupBy,
		// then we need a cut for the select expressions.
		// For SELECT *, cutter is nil.
		selector, err := convertSQLSelect(selection)
		if err != nil {
			return nil, err
		}
		if selector != nil {
			ops = append(ops, selector)
		}
	}
	if sql.OrderBy != nil {
		keys, err := semExprs(scope, sql.OrderBy.Keys)
		if err != nil {
			return nil, err
		}
		ops = append(ops, sortByMulti(keys, sql.OrderBy.Order))
	}
	if sql.Limit != 0 {
		p := &dag.Head{
			Kind:  "Head",
			Count: sql.Limit,
		}
		ops = append(ops, p)
	}
	if len(ops) == 0 {
		ops = []dag.Op{&dag.Pass{"Pass"}}
	}
	return wrap(ops), nil
}

func isID(e ast.Expr) (string, bool) {
	if id, ok := e.(*ast.ID); ok {
		return id.Name, true
	}
	return "", false
}

func liftWhereFilter(aliasID string, where dag.Expr, joins []ast.SQLJoin) *dag.Filter {
	for _, join := range joins {
		// For now, if there are multiple join aliases, be pessimistic
		// and don't try to lift the where.  We can fix this later.
		if _, ok := isID(join.Alias); !ok {
			return nil
		}
	}
	eligible := eligiblePred(aliasID, where)
	if eligible == nil {
		return nil
	}
	return &dag.Filter{
		Kind: "Filter",
		Expr: eligible,
	}
}

func eligiblePred(aliasID string, e dag.Expr) dag.Expr {
	switch e := e.(type) {
	case *dag.UnaryExpr:
		if operand := eligiblePred(aliasID, e.Operand); operand != nil {
			return &dag.UnaryExpr{
				Kind:    "UnaryExpr",
				Op:      "!",
				Operand: operand,
			}
		}
	case *zed.Primitive:
		return e
	case *dag.Dot:
		// A field reference of the form <aliasID>.x is eligible
		// as the field x because, when lifted it was called x.
		return eligibleFieldRef(aliasID, e)
	case *dag.BinaryExpr:
		lhs := eligiblePred(aliasID, e.LHS)
		rhs := eligiblePred(aliasID, e.RHS)
		if e.Op == "or" {
			if lhs != nil && rhs != nil {
				return &dag.BinaryExpr{
					Kind: "BinaryExpr",
					Op:   e.Op,
					LHS:  lhs,
					RHS:  rhs,
				}
			}
			return nil
		}
		if e.Op == "and" {
			if lhs == nil {
				return rhs
			}
			if rhs == nil {
				return lhs
			}
			return &dag.BinaryExpr{
				Kind: "BinaryExpr",
				Op:   e.Op,
				LHS:  lhs,
				RHS:  rhs,
			}
		}
		if lhs != nil && rhs != nil {
			return &dag.BinaryExpr{
				Kind: "BinaryExpr",
				Op:   e.Op,
				LHS:  lhs,
				RHS:  rhs,
			}
		}
	}
	return nil
}

func eligibleFieldRef(aliasID string, e *dag.Dot) dag.Expr {
	lhs, ok := e.LHS.(*dag.Dot)
	if ok && dag.IsRoot(lhs) && lhs.RHS == aliasID {
		return &dag.Dot{
			Kind: "Dot",
			LHS:  dag.Root,
			RHS:  e.RHS,
		}
	}
	return nil
}

func convertSQLTableRef(scope *Scope, e ast.Expr) (dag.Op, error) {
	converted, err := semExpr(scope, e)
	if err != nil {
		return nil, err
	}
	// If an identifier name is given with no definition for that name,
	// then convert it to a type name as it is otherwise expected that
	// the type name will be defined by the data stream.
	if id, ok := dag.RootField(converted); ok && scope.Lookup(id) == nil {
		converted = &zed.TypeValue{
			Kind: "TypeValue",
			Value: &zed.TypeName{
				Kind: "TypeName",
				Name: id,
			},
		}
	}
	return &dag.Filter{
		Kind: "Filter",
		Expr: &dag.Call{
			Kind: "Call",
			Name: "is",
			Args: []dag.Expr{converted},
		},
	}, nil
}

func convertSQLAlias(scope *Scope, e ast.Expr) (*dag.Cut, string, error) {
	if e == nil {
		return nil, "", nil
	}
	fld, err := semField(scope, e)
	if err != nil {
		return nil, "", fmt.Errorf("illegal alias: %w", err)
	}
	var id string
	if idExpr, ok := e.(*ast.ID); ok {
		id = idExpr.Name
	}
	assignment := dag.Assignment{
		Kind: "Assignment",
		LHS:  fld,
		RHS:  dag.Root,
	}
	return &dag.Cut{
		Kind: "Cut",
		Args: []dag.Assignment{assignment},
	}, id, nil
}

func wrap(ops []dag.Op) dag.Op {
	if len(ops) == 0 {
		return nil
	}
	if len(ops) == 1 {
		return ops[0]
	}
	return &dag.Sequential{
		Kind: "Sequential",
		Ops:  ops,
	}
}

func convertSQLJoins(scope *Scope, fromPath []dag.Op, joins []ast.SQLJoin) ([]dag.Op, error) {
	left := fromPath
	for _, right := range joins {
		var err error
		left, err = convertSQLJoin(scope, left, right)
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

// For now, each joining table is on the right...
// We don't have logic to not care about the side of the JOIN ON keys...
func convertSQLJoin(scope *Scope, leftPath []dag.Op, sqlJoin ast.SQLJoin) ([]dag.Op, error) {
	if sqlJoin.Alias == nil {
		return nil, errors.New("JOIN currently requires alias, e.g., JOIN <type> <alias> (will be fixed soon)")
	}
	leftKey, err := semExpr(scope, sqlJoin.LeftKey)
	if err != nil {
		return nil, err
	}
	leftPath = append(leftPath, sortBy(leftKey))
	joinFilter, err := convertSQLTableRef(scope, sqlJoin.Table)
	if err != nil {
		return nil, err
	}
	rightPath := []dag.Op{joinFilter}
	cut, aliasID, err := convertSQLAlias(scope, sqlJoin.Alias)
	if err != nil {
		return nil, errors.New("JOIN alias must be a name")
	}
	rightPath = append(rightPath, cut)
	rightKey, err := semExpr(scope, sqlJoin.RightKey)
	if err != nil {
		return nil, err
	}
	rightPath = append(rightPath, sortBy(rightKey))
	fork := &dag.Parallel{
		Kind: "Parallel",
		Ops:  []dag.Op{wrap(leftPath), wrap(rightPath)},
	}
	alias := dag.Assignment{
		Kind: "Assignment",
		LHS:  &dag.Path{"Path", field.New(aliasID)},
		RHS:  &dag.Path{"Path", field.New(aliasID)},
	}
	join := &dag.Join{
		Kind:     "Join",
		Style:    sqlJoin.Style,
		LeftKey:  leftKey,
		RightKey: rightKey,
		Args:     []dag.Assignment{alias},
	}
	return []dag.Op{fork, join}, nil
}

func sortBy(e dag.Expr) *dag.Sort {
	return sortByMulti([]dag.Expr{e}, "asc")
}

func sortByMulti(keys []dag.Expr, order string) *dag.Sort {
	// XXX ast.Sort should take a zbuf.Order instead of an in direction
	// (and probably this constant should move out of zbuf and into ast)
	// See issue #2397
	direction := 1
	if order == "desc" {
		direction = -1
	}
	return &dag.Sort{
		Kind:    "Sort",
		Args:    keys,
		SortDir: direction,
	}
}

func convertSQLSelect(selection sqlSelection) (dag.Op, error) {
	// This is a straight select without a group-by.
	// If all the expressions are aggregators, then we build a group-by.
	// If it's mixed, we return an error.  Otherwise, we do a simple cut.
	var nagg int
	for _, p := range selection {
		if p.agg != nil {
			nagg++
		}
	}
	if nagg == 0 {
		return selection.cut(), nil
	}
	if nagg != len(selection) {
		return nil, errors.New("cannot mix aggregations and non-aggregations without a GROUP BY")
	}
	// Note here that we reconstruct the group-by aggregators instead of
	// using the assignments in ast.SqlExpression.Select since the SQL peg
	// parser does not know whether they are aggregators or function calls,
	// but the sqlPick elements have this determined.  So we take the LHS
	// from the original expression and mix it with the agg that was put
	// in sqlPick.
	var assignments []dag.Assignment
	for _, p := range selection {
		a := dag.Assignment{
			Kind: "Assignment",
			LHS:  p.assignment.LHS,
			RHS:  p.agg,
		}
		assignments = append(assignments, a)
	}
	return &dag.Summarize{
		Kind: "Summarize",
		Aggs: assignments,
	}, nil
}

func convertSQLGroupBy(scope *Scope, groupByKeys []ast.Expr, selection sqlSelection) (dag.Op, error) {
	var keys []field.Static
	for _, key := range groupByKeys {
		name, err := sqlField(scope, key)
		if err != nil {
			return nil, fmt.Errorf("bad GROUP BY key: %w", err)
		}
		keys = append(keys, name)
	}
	// Make sure all group-by keys are in the selection.
	all := selection.fields()
	for _, key := range keys {
		//XXX fix this for select *?
		if !key.In(all) {
			if key.HasPrefixIn(all) {
				return nil, fmt.Errorf("'%s': GROUP BY key cannot be a sub-field of the selected value", key)
			}
			return nil, fmt.Errorf("'%s': GROUP BY key not in selection", key)
		}
	}
	// Make sure all scalars are in the group-by keys.
	scalars := selection.scalars()
	for _, f := range scalars.fields() {
		if !f.In(keys) {
			return nil, fmt.Errorf("'%s': selected expression is missing from GROUP BY clause (and is not an aggregation)", f)
		}
	}
	// Now that the selection and keys have been checked, build the
	// key expressions from the scalars of the select and build the
	// aggregators (aka reducers) from the aggregation functions present
	// in the select clause.
	var keyExprs []dag.Assignment
	for _, p := range scalars {
		keyExprs = append(keyExprs, p.assignment)
	}
	var aggExprs []dag.Assignment
	for _, p := range selection.aggs() {
		aggExprs = append(aggExprs, dag.Assignment{
			LHS: p.assignment.LHS,
			RHS: p.agg,
		})
	}
	// XXX how to override limit for spills?
	return &dag.Summarize{
		Kind: "Summarize",
		Keys: keyExprs,
		Aggs: aggExprs,
	}, nil
}

// A sqlPick is one column of a select statement.  We bookkeep here whether
// a column is a scalar expression or an aggregation by looking up the function
// name and seeing if it's an aggregator or not.  We also infer the column
// names so we can do SQL error checking relating the selections to the group-by
// keys, something that is not needed in Z.
type sqlPick struct {
	name       field.Static
	agg        *dag.Agg
	assignment dag.Assignment
}

type sqlSelection []sqlPick

func newSQLSelection(scope *Scope, assignments []ast.Assignment) (sqlSelection, error) {
	// Make a cut from a SQL select.  This should just work
	// without having to track identifier names of columns because
	// the transformations will all relable data from stage to stage
	// and Select names refer to the names at the last stage of
	// the table.
	var s sqlSelection
	for _, a := range assignments {
		name, err := deriveAs(scope, a)
		if err != nil {
			return nil, err
		}
		agg, err := isAgg(scope, a.RHS)
		if err != nil {
			return nil, err
		}
		assignment, err := semAssignment(scope, a)
		if err != nil {
			return nil, err
		}
		s = append(s, sqlPick{name, agg, assignment})
	}
	return s, nil
}

func (s sqlSelection) fields() []field.Static {
	var fields []field.Static
	for _, p := range s {
		fields = append(fields, p.name)
	}
	return fields
}

func (s sqlSelection) aggs() sqlSelection {
	var aggs sqlSelection
	for _, p := range s {
		if p.agg != nil {
			aggs = append(aggs, p)
		}
	}
	return aggs
}

func (s sqlSelection) scalars() sqlSelection {
	var scalars sqlSelection
	for _, p := range s {
		if p.agg == nil {
			scalars = append(scalars, p)
		}
	}
	return scalars
}

func (s sqlSelection) cut() *dag.Cut {
	if len(s) == 0 {
		return nil
	}
	var a []dag.Assignment
	for _, p := range s {
		a = append(a, p.assignment)
	}
	return &dag.Cut{
		Kind: "Cut",
		Args: a,
	}
}

func isAgg(scope *Scope, e ast.Expr) (*dag.Agg, error) {
	call, ok := e.(*ast.Call)
	if !ok {
		return nil, nil
	}
	if _, err := agg.NewPattern(call.Name); err != nil {
		return nil, nil
	}
	var arg ast.Expr
	if len(call.Args) > 1 {
		return nil, fmt.Errorf("%s: wrong number of arguments", call.Name)
	}
	if len(call.Args) == 1 {
		arg = call.Args[0]
	}
	var dagArg dag.Expr
	if arg != nil {
		var err error
		dagArg, err = semExpr(scope, arg)
		if err != nil {
			return nil, err
		}
	}
	return &dag.Agg{
		Kind: "Agg",
		Name: call.Name,
		Expr: dagArg,
	}, nil
}

func deriveAs(scope *Scope, a ast.Assignment) (field.Static, error) {
	sa, err := semAssignment(scope, a)
	if err != nil {
		return nil, fmt.Errorf("AS clause of SELECT: %w", err)
	}
	f, ok := sa.LHS.(*dag.Path)
	if !ok {
		return nil, fmt.Errorf("AS clause not a field: %w", err)
	}
	return f.Name, nil
}

func sqlField(scope *Scope, e ast.Expr) (field.Static, error) {
	name, err := semField(scope, e)
	if err != nil {
		return nil, err
	}
	if f, ok := name.(*dag.Path); ok {
		return f.Name, nil
	}
	return nil, errors.New("expression is not a field reference")
}
