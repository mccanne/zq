package zfmt

import (
	"fmt"

	"github.com/brimsec/zq/compiler/ast"
)

func Debug(p ast.Proc) string {
	d := &debug{formatter{tab: 2}}
	d.proc(p)
	d.flush()
	return d.String()
}

type debug struct {
	formatter
}

func (d *debug) open(args ...interface{}) {
	d.write("(")
	d.formatter.open(args...)
}

func (d *debug) close() {
	d.cont(")")
	d.formatter.close()
}

func (d *debug) assignments(assignments []ast.Assignment) {
	first := true
	for _, a := range assignments {
		if !first {
			d.ret()
		} else {
			first = false
		}
		d.open("= ")
		d.expr(a.LHS)
		d.write(" ")
		d.expr(a.RHS)
		d.close()
	}
}

func (d *debug) exprs(exprs []ast.Expression) {
	first := true
	for _, e := range exprs {
		if first {
			first = false
		} else {
			d.space()
		}
		d.expr(e)
	}
}

func (d *debug) expr(e ast.Expression) {
	switch e := e.(type) {
	case nil:
		d.write("(null)")
	case *ast.Reducer:
		d.open(e.Operator)
		d.expr(e.Expr)
		if e.Where != nil {
			d.open("where ")
			d.expr(e.Where)
			d.close()
		}
		d.close()
	case *ast.Literal:
		d.open("literal ")
		d.write(e.Type)
		d.write(" ")
		d.write(e.Value)
		d.close()
	case *ast.Identifier:
		// If the identifier refers to a named variable in scope (like "$"),
		// then return a Var expression referring to the pointer to the value.
		// Note that constants may be accessed this way too by entering their
		// names into the global (outermost) scope in the Scope entity.
		d.open("id ")
		d.write(e.Name)
		d.close()
	case *ast.RootRecord:
		d.write(".")
	case *ast.UnaryExpression:
		d.open(e.Operator)
		d.write(" ")
		d.expr(e.Operand)
		d.close()
	case *ast.SelectExpression:
		d.write("(select)")
	case *ast.BinaryExpression:
		d.open(e.Operator)
		d.space()
		d.expr(e.LHS)
		d.space()
		d.expr(e.RHS)
		d.close()
	case *ast.ConditionalExpression:
		d.open("cond ")
		d.expr(e.Condition)
		d.space()
		d.expr(e.Then)
		d.space()
		d.expr(e.Else)
		d.close()
	case *ast.FunctionCall:
		d.open(e.Function)
		d.space()
		d.exprs(e.Args)
		d.close()
	case *ast.CastExpression:
		d.open("cast %s ", e.Type)
		d.expr(e.Expr)
		d.close()
	case *ast.Search:
		d.open("search")
		d.write(" text %s", e.Text)
		d.ret()
		d.write(" val %s %s", e.Value.Type, e.Value.Value)
		d.close()
	default:
		d.open("(unknown expr %T)", e)
		d.close()
		d.ret()
	}
}

func (d *debug) procs(procs []ast.Proc) {
	for _, p := range procs {
		d.proc(p)
		d.ret()
	}
}

func (d *debug) proc(p ast.Proc) {
	switch p := p.(type) {
	case *ast.SequentialProc:
		d.open("seq")
		d.ret()
		d.procs(p.Procs)
		d.close()
	case *ast.ParallelProc:
		d.open("par")
		d.ret()
		d.procs(p.Procs)
		d.close()
	case *ast.GroupByProc:
		d.open("groupby dur=%d dir=%d limit=%d", p.Duration, p.InputSortDir, p.Limit)
		if p.ConsumePart {
			d.write(" partials-in")
		}
		if p.EmitPart {
			d.write(" partials-out")
		}
		d.ret()
		d.open("keys")
		d.assignments(p.Keys)
		d.close()
		d.ret()
		d.open("aggs")
		d.assignments(p.Reducers)
		d.close()
		d.close()
	case *ast.CutProc:
		d.open("cut")
		d.ret()
		d.assignments(p.Fields)
		d.close()
	case *ast.PickProc:
		d.open("pick")
		d.ret()
		d.assignments(p.Fields)
		d.close()
	case *ast.DropProc:
		d.open("drop")
		d.ret()
		d.exprs(p.Fields)
		d.close()
	case *ast.SortProc:
		d.open(fmt.Sprintf("sort dir=%d nf=%t ", p.SortDir, p.NullsFirst))
		d.ret()
		d.exprs(p.Fields)
		d.close()
	case *ast.HeadProc:
		d.open("head %d", p.Count)
		d.close()
	case *ast.TailProc:
		d.open("tail %d", p.Count)
		d.close()
	case *ast.UniqProc:
		d.open("uniq")
		if p.Cflag {
			d.write(" -c")
		}
		d.close()
	case *ast.PassProc:
		d.open("pass")
	case *ast.FilterProc:
		d.open("filter ")
		d.expr(p.Filter)
		d.close()
	case *ast.TopProc:
		d.open("top limit=%d flush=%t ", p.Limit, p.Flush)
		d.ret()
		d.exprs(p.Fields)
		d.close()
	case *ast.PutProc:
		d.open("put")
		d.ret()
		d.assignments(p.Clauses)
		d.close()
	case *ast.RenameProc:
		d.open("rename")
		d.ret()
		d.assignments(p.Fields)
		d.close()
	case *ast.FuseProc:
		d.open("fuse")
		d.close()
	case *ast.FunctionCall:
		d.open(p.Function)
		d.space()
		d.exprs(p.Args)
		d.close()
	case *ast.JoinProc:
		d.open("join on ")
		d.expr(p.LeftKey)
		d.write(" = ")
		d.expr(p.RightKey)
		d.ret()
		d.open("join-cut ")
		d.assignments(p.Clauses)
		d.close()
		d.close()
	//case *ast.SqlExpression:
	//	//XXX TBD
	//	d.open("sql")
	//	d.close()
	default:
		d.open("unknown proc: %T", p)
		d.close()
	}
}
