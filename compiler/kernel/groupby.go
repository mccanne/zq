package kernel

import (
	"errors"
	"fmt"

	"github.com/brimdata/zed/compiler/ast/dag"
	"github.com/brimdata/zed/expr"
	"github.com/brimdata/zed/field"
	"github.com/brimdata/zed/proc"
	"github.com/brimdata/zed/proc/groupby"
	"github.com/brimdata/zed/zson"
)

func compileGroupBy(pctx *proc.Context, scope *Scope, parent proc.Interface, summarize *dag.Summarize) (*groupby.Proc, error) {
	keys, err := compileAssignments(summarize.Keys, pctx.Zctx, scope)
	if err != nil {
		return nil, err
	}
	names, reducers, err := compileAggs(summarize.Aggs, scope, pctx.Zctx)
	if err != nil {
		return nil, err
	}
	return groupby.New(pctx, parent, keys, names, reducers, summarize.Limit, summarize.InputSortDir, summarize.PartialsIn, summarize.PartialsOut)
}

func compileAggs(assignments []dag.Assignment, scope *Scope, zctx *zson.Context) ([]field.Static, []*expr.Aggregator, error) {
	names := make([]field.Static, 0, len(assignments))
	aggs := make([]*expr.Aggregator, 0, len(assignments))
	for _, assignment := range assignments {
		name, agg, err := compileAgg(zctx, scope, assignment)
		if err != nil {
			return nil, nil, err
		}
		aggs = append(aggs, agg)
		names = append(names, name)
	}
	return names, aggs, nil
}

func compileAgg(zctx *zson.Context, scope *Scope, assignment dag.Assignment) (field.Static, *expr.Aggregator, error) {
	aggAST, ok := assignment.RHS.(*dag.Agg)
	if !ok {
		return nil, nil, errors.New("aggregator is not an aggregation expression")
	}
	aggName := aggAST.Name
	var err error
	var arg expr.Evaluator
	if aggAST.Expr != nil {
		arg, err = compileExpr(zctx, nil, aggAST.Expr)
		if err != nil {
			return nil, nil, err
		}
	}
	// If there is a reducer assignment, the LHS is non-nil and we
	// compile.  Otherwise, we infer an LHS top-level field name from
	// the name of reducer function.
	var lhs field.Static
	if assignment.LHS == nil {
		lhs = field.New(aggName)
	} else {
		lhs, err = compileLval(assignment.LHS)
		if err != nil {
			return nil, nil, fmt.Errorf("lhs of aggregation: %w", err)
		}
	}
	var where expr.Filter
	if aggAST.Where != nil {
		where, err = CompileFilter(zctx, scope, aggAST.Where)
		if err != nil {
			return nil, nil, err
		}
	}
	m, err := expr.NewAggregator(aggName, arg, where)
	return lhs, m, err
}
