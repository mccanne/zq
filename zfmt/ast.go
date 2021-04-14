package zfmt

import (
	"strings"

	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/compiler/ast/zed"
)

func AST(p ast.Proc) string {
	c := &canon{canonZed: canonZed{formatter{tab: 2}}, head: true, first: true}
	c.proc(p)
	c.flush()
	return c.String()
}

type canon struct {
	canonZed
	head  bool
	first bool
}

func (c *canon) open(args ...interface{}) {
	c.formatter.open(args...)
}

func (c *canon) close() {
	c.formatter.close()
}

func (c *canon) assignments(assignments []ast.Assignment) {
	for k, a := range assignments {
		if k > 0 {
			c.write(",")
		}
		if a.LHS != nil {
			c.expr(a.LHS, false)
			c.write("=")
		}
		c.expr(a.RHS, false)
	}
}

func (c *canon) exprs(exprs []ast.Expr) {
	for k, e := range exprs {
		if k > 0 {
			c.write(", ")
		}
		c.expr(e, false)
	}
}

func (c *canon) expr(e ast.Expr, paren bool) {
	switch e := e.(type) {
	case nil:
		c.write("null")
	case *ast.Agg:
		c.write("%s(", e.Name)
		if e.Expr != nil {
			c.expr(e.Expr, false)
		}
		c.write(")")
		if e.Where != nil {
			c.write(" where ")
			c.expr(e.Where, false)
		}
	case *zed.Primitive:
		c.literal(*e)
	case *ast.ID:
		c.write(e.Name)
	case *ast.Root:
		c.write(".")
	case *ast.UnaryExpr:
		c.space()
		c.write(e.Op)
		c.expr(e.Operand, true)
	case *ast.SelectExpr:
		c.write("TBD:select")
	case *ast.BinaryExpr:
		c.binary(e)
	case *ast.Conditional:
		c.write("(")
		c.expr(e.Cond, true)
		c.write(") ? ")
		c.expr(e.Then, false)
		c.write(" : ")
		c.expr(e.Else, false)
	case *ast.Call:
		c.write("%s(", e.Name)
		c.exprs(e.Args)
		c.write(")")
	case *ast.Cast:
		c.expr(e.Expr, false)
		c.open(":%s", e.Type)
	case *ast.Search:
		c.write("match(")
		c.literal(e.Value)
		c.write(")")
	case *ast.SQLExpr:
		c.sql(e)
	case *zed.TypeValue:
		c.write("type(")
		c.typ(e.Value)
		c.write(")")
	default:
		c.open("(unknown expr %T)", e)
		c.close()
		c.ret()
	}
}

func (c *canon) binary(e *ast.BinaryExpr) {
	switch e.Op {
	case ".":
		if !isRoot(e.LHS) {
			c.expr(e.LHS, false)
			c.write(".")
		}
		c.expr(e.RHS, false)
	case "[":
		if isRoot(e.LHS) {
			c.write(".")
		} else {
			c.expr(e.LHS, false)
		}
		c.write("[")
		c.expr(e.RHS, false)
		c.write("]")
	case "in", "and":
		c.expr(e.LHS, false)
		c.write(" %s ", e.Op)
		c.expr(e.RHS, false)
	case "or":
		c.expr(e.LHS, true)
		c.write(" %s ", e.Op)
		c.expr(e.RHS, true)
	default:
		// do need parens calc
		c.expr(e.LHS, true)
		c.write("%s", e.Op)
		c.expr(e.RHS, true)
	}
}

func (c *canon) sql(e *ast.SQLExpr) {
	if e.Select == nil {
		c.write(" SELECT *")
	} else {
		c.write(" SELECT")
		c.assignments(e.Select)
	}
}

func isRoot(e ast.Expr) bool {
	if _, ok := e.(*ast.Root); ok {
		return true
	}
	return false
}

func (c *canon) next() {
	if c.first {
		c.first = false
	} else {
		c.write("\n")
	}
	c.needRet = false
	c.writeTab()
	if c.head {
		c.head = false
	} else {
		c.write("| ")
	}
}

func (c *canon) proc(p ast.Proc) {
	switch p := p.(type) {
	case *ast.Sequential:
		for _, p := range p.Procs {
			c.proc(p)
		}
	case *ast.Parallel:
		c.next()
		c.open("split (")
		for _, p := range p.Procs {
			c.ret()
			c.write("=>")
			c.open()
			c.head = true
			c.proc(p)
			c.close()
		}
		c.close()
		c.ret()
		c.flush()
		c.write(")")
		if p.MergeBy != nil {
			c.write(" merge-by ")
			c.fieldpath(p.MergeBy)
		}
		if p.MergeReverse {
			c.write(" rev")
		}
	case *ast.Const:
		c.write("const %s=", p.Name)
		c.expr(p.Expr, false)
		c.ret()
		c.flush()
	case *ast.TypeProc:
		c.write("type %s=", p.Name)
		c.typ(p.Type)
		c.ret()
		c.flush()
	case *ast.SQLExpr:
		c.next()
		c.open("SELECT ")
		if p.Select == nil {
			c.write("*")
		} else {
			c.assignments(p.Select)
		}
		if p.From != nil {
			c.ret()
			c.write("FROM ")
			c.expr(p.From.Table, false)
			if p.From.Alias != nil {
				c.write(" AS ")
				c.expr(p.From.Alias, false)
			}
		}
		for _, join := range p.Joins {
			c.ret()
			switch join.Style {
			case "left":
				c.write("LEFT ")
			case "right":
				c.write("RIGHT ")
			}
			c.write("JOIN ")
			c.expr(join.Table, false)
			if join.Alias != nil {
				c.write(" AS ")
				c.expr(join.Alias, false)
			}
			c.write(" ON ")
			c.expr(join.LeftKey, false)
			c.write("=")
			c.expr(join.RightKey, false)
		}
		if p.Where != nil {
			c.ret()
			c.write("WHERE ")
			c.expr(p.Where, false)
		}
		if p.GroupBy != nil {
			c.ret()
			c.write("GROUP BY ")
			c.exprs(p.GroupBy)
		}
		if p.Having != nil {
			c.ret()
			c.write("HAVING ")
			c.expr(p.Having, false)
		}
		if p.OrderBy != nil {
			c.ret()
			c.write("ORDER BY ")
			c.exprs(p.OrderBy.Keys)
			if p.OrderBy.Order != "" {
				c.write(" ")
				c.write(strings.ToUpper(p.OrderBy.Order))
			}
		}
		if p.Limit != 0 {
			c.ret()
			c.write("LIMIT %d", p.Limit)
		}
	case *ast.Summarize:
		c.next()
		c.open("summarize")
		if p.Duration != nil {
			c.write(" every ")
			c.literal(*p.Duration)
		}
		c.ret()
		c.open()
		c.assignments(p.Aggs)
		if len(p.Keys) != 0 {
			c.write(" by ")
			c.assignments(p.Keys)
		}
		if p.Limit != 0 {
			c.write(" -with limit %d", p.Limit)
		}
		c.close()
		c.close()
	case *ast.Cut:
		c.next()
		c.write("cut ")
		c.assignments(p.Args)
	case *ast.Pick:
		c.next()
		c.open("pick ")
		c.assignments(p.Args)
	case *ast.Drop:
		c.next()
		c.write("drop ")
		c.exprs(p.Args)
	case *ast.Sort:
		c.next()
		c.write("sort")
		if p.SortDir < 0 {
			c.write(" -r")
		}
		if p.NullsFirst {
			c.write(" -nulls first")
		}
		if len(p.Args) > 0 {
			c.space()
			c.exprs(p.Args)
		}
	case *ast.Head:
		c.next()
		c.write("head %d", p.Count)
	case *ast.Tail:
		c.next()
		c.write("tail %d", p.Count)
	case *ast.Uniq:
		c.next()
		c.write("uniq")
		if p.Cflag {
			c.write(" -c")
		}
	case *ast.Pass:
		c.next()
		c.write("pass")
	case *ast.Filter:
		c.next()
		c.open("filter ")
		if isTrue(p.Expr) {
			c.write("*")
		} else {
			c.expr(p.Expr, false)
		}
		c.close()
	case *ast.Top:
		c.next()
		c.write("top limit=%d flush=%t ", p.Limit, p.Flush)
		c.exprs(p.Args)
	case *ast.Put:
		c.next()
		c.write("put ")
		c.assignments(p.Args)
	case *ast.Rename:
		c.next()
		c.write("rename ")
		c.assignments(p.Args)
	case *ast.Fuse:
		c.next()
		c.write("fuse")
	case *ast.Call:
		c.next()
		c.write("%s(", p.Name)
		c.exprs(p.Args)
		c.write(")")
	case *ast.Join:
		c.next()
		c.open("join on ")
		c.expr(p.LeftKey, false)
		c.write("=")
		c.expr(p.RightKey, false)
		c.ret()
		c.open("join-cut ")
		c.assignments(p.Args)
		c.close()
		c.close()
	//case *ast.SqlExpression:
	//	//XXX TBD
	//	c.open("sql")
	//	c.close()
	default:
		c.open("unknown proc: %T", p)
		c.close()
	}
}

func isTrue(e ast.Expr) bool {
	if p, ok := e.(*zed.Primitive); ok {
		return p.Type == "bool" && p.Text == "true"
	}
	return false
}
