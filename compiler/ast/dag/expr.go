package dag

import "github.com/brimdata/zed/compiler/ast/zed"

type Expr interface {
	ExprDAG()
}

// Exprs

type (
	ArrayExpr struct {
		Kind  string `json:"kind" unpack:""`
		Exprs []Expr `json:"exprs"`
	}
	BinaryExpr struct {
		Kind string `json:"kind" unpack:""`
		Op   string `json:"op"`
		LHS  Expr   `json:"lhs"`
		RHS  Expr   `json:"rhs"`
	}
	Call struct {
		Kind string `json:"kind" unpack:""`
		Name string `json:"name"`
		Args []Expr `json:"args"`
	}
	Cast struct {
		Kind string   `json:"kind" unpack:""`
		Expr Expr     `json:"expr"`
		Type zed.Type `json:"type"`
	}
	Conditional struct {
		Kind string `json:"kind" unpack:""`
		Cond Expr   `json:"cond"`
		Then Expr   `json:"then"`
		Else Expr   `json:"else"`
	}
	Dot struct {
		Kind string `json:"kind" unpack:""`
		LHS  Expr   `json:"lhs"`
		RHS  string `json:"rhs"`
	}
	MapExpr struct {
		Kind    string  `json:"kind" unpack:""`
		Entries []Entry `json:"entries"`
	}
	Path struct {
		Kind string   `json:"kind" unpack:""`
		Name []string `json:"name"`
	}
	RecordExpr struct {
		Kind   string  `json:"kind" unpack:""`
		Fields []Field `json:"fields"`
	}
	Ref struct {
		Kind string `json:"kind" unpack:""`
		Name string `json:"name"`
	}
	RegexpMatch struct {
		Kind    string `json:"kind" unpack:""`
		Pattern string `json:"pattern"`
		Expr    Expr   `json:"expr"`
	}
	RegexpSearch struct {
		Kind    string `json:"kind" unpack:""`
		Pattern string `json:"pattern"`
	}
	Search struct {
		Kind  string        `json:"kind" unpack:""`
		Text  string        `json:"text"`
		Value zed.Primitive `json:"value"` //XXX search should be extended to complex types
	}
	SelectExpr struct {
		Kind      string `json:"kind" unpack:""`
		Selectors []Expr `json:"selectors"`
		Methods   []Call `json:"methods"`
	}
	SeqExpr struct {
		Kind      string   `json:"kind" unpack:""`
		Name      string   `json:"name"`
		Selectors []Expr   `json:"selectors"`
		Methods   []Method `json:"methods"`
	}
	SetExpr struct {
		Kind  string `json:"kind" unpack:""`
		Exprs []Expr `json:"exprs"`
	}
	UnaryExpr struct {
		Kind    string `json:"kind" unpack:""`
		Op      string `json:"op"`
		Operand Expr   `json:"operand"`
	}
)

// Various Expr fields.

type (
	Field struct {
		Name  string `json:"name"`
		Value Expr   `json:"value"`
	}
	Entry struct {
		Key   Expr `json:"key"`
		Value Expr `json:"value"`
	}
)

func (*Agg) ExprDAG()          {}
func (*ArrayExpr) ExprDAG()    {}
func (*BinaryExpr) ExprDAG()   {}
func (*Call) ExprDAG()         {}
func (*Cast) ExprDAG()         {}
func (*Conditional) ExprDAG()  {}
func (*Dot) ExprDAG()          {}
func (*MapExpr) ExprDAG()      {}
func (*Path) ExprDAG()         {}
func (*RecordExpr) ExprDAG()   {}
func (*Ref) ExprDAG()          {}
func (*RegexpMatch) ExprDAG()  {}
func (*RegexpSearch) ExprDAG() {}
func (*SelectExpr) ExprDAG()   {}
func (*Search) ExprDAG()       {}
func (*SeqExpr) ExprDAG()      {}
func (*SetExpr) ExprDAG()      {}
func (*UnaryExpr) ExprDAG()    {}

var Root = &Path{Kind: "Path", Name: []string{}}

func IsRoot(e Expr) bool {
	if p, ok := e.(*Path); ok {
		return len(p.Name) == 0
	}
	return false
}

func RootField(e Expr) (string, bool) {
	if b, ok := e.(*Path); ok {
		if len(b.Name) == 1 {
			return b.Name[0], true
		}
	}
	return "", false
}

func IsRootField(e Expr) bool {
	_, ok := RootField(e)
	return ok
}
