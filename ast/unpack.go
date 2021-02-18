package ast

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/brimsec/zq/pkg/joe"
	"github.com/mitchellh/mapstructure"
)

type Unpacker interface {
	Unpack(string, joe.Interface) (Proc, error)
}

func unpackProgram(node joe.Interface) (*Program, error) {
	op, err := getString(node, "op")
	if err != nil {
		return nil, err
	}
	if op != "Program" {
		return nil, errors.New("ast.unpackProgram: op not 'Program'")
	}
	entry, err := node.Get("entry")
	if err != nil {
		return nil, errors.New("ast.unpackProgram: entry field is missing")
	}
	program := &Program{}
	program.Entry, err = unpackProc(nil, entry)
	if err != nil {
		return nil, err
	}
	return program, nil
}

func unpackProcs(custom Unpacker, node joe.Interface) ([]Proc, error) {
	if node == nil {
		return nil, errors.New("ast.unpackProcs: procs field is missing")
	}
	procList, err := node.Get("procs")
	if err != nil {
		return nil, fmt.Errorf("ast.unpackProcs: procs field is missing")
	}
	a, ok := procList.(joe.Array)
	if !ok {
		return nil, fmt.Errorf("ast.unpackProcs: procs field is not an array")
	}
	procs := make([]Proc, 0, len(a))
	for _, item := range a {
		proc, err := unpackProc(custom, item)
		if err != nil {
			return nil, err
		}
		procs = append(procs, proc)
	}
	return procs, nil
}

func getString(node joe.Interface, field string) (string, error) {
	item, err := node.Get(field)
	if err != nil {
		return "", fmt.Errorf("AST is missing %s field: %s", node, field)
	}
	s, err := item.String()
	if err != nil {
		return "", fmt.Errorf("AST field %s of %s is not a string", field, node)
	}
	return s, nil
}

func unpackProc(custom Unpacker, node joe.Interface) (Proc, error) {
	if node == nil {
		return nil, errors.New("bad AST: missing proc field")
	}
	op, err := getString(node, "op")
	if err != nil {
		return nil, err
	}
	if custom != nil {
		p, err := custom.Unpack(op, node)
		if err != nil {
			return nil, err
		}
		if p != nil {
			return p, nil
		}
	}

	switch op {
	case "SequentialProc":
		procs, err := unpackProcs(custom, node)
		if err != nil {
			return nil, err
		}
		return &SequentialProc{Procs: procs}, nil
	case "ParallelProc":
		procs, err := unpackProcs(custom, node)
		if err != nil {
			return nil, err
		}
		return &ParallelProc{Procs: procs}, nil
	case "SortProc":
		a, _ := node.Get("fields")
		fields, err := unpackExprs(a)
		if err != nil {
			return nil, err
		}
		return &SortProc{Fields: fields}, nil
	case "CutProc":
		a, _ := node.Get("fields")
		fas, err := unpackAssignments(a)
		if err != nil {
			return nil, err
		}
		return &CutProc{Fields: fas}, nil
	case "PickProc":
		a, _ := node.Get("fields")
		fas, err := unpackAssignments(a)
		if err != nil {
			return nil, err
		}
		return &PickProc{Fields: fas}, nil
	case "DropProc":
		a, _ := node.Get("fields")
		fields, err := unpackExprs(a)
		if err != nil {
			return nil, err
		}
		return &DropProc{Fields: fields}, nil
	case "HeadProc":
		return &HeadProc{}, nil
	case "TailProc":
		return &TailProc{}, nil
	case "FilterProc":
		f, err := node.Get("filter")
		if err != nil {
			return nil, errors.New("ast filter proc: missing filter field")
		}
		filter, err := unpackExpression(f)
		if err != nil {
			return nil, err
		}
		return &FilterProc{Filter: filter}, nil
	case "FunctionCall":
		// A FuncionCall can appear in proc context too.
		args, err := unpackArgs(node)
		if err != nil {
			return nil, err
		}
		return &FunctionCall{Args: args}, nil
	case "PutProc":
		a, _ := node.Get("clauses")
		clauses, err := unpackAssignments(a)
		if err != nil {
			return nil, err
		}
		return &PutProc{Clauses: clauses}, nil
	case "RenameProc":
		a, _ := node.Get("fields")
		fas, err := unpackAssignments(a)
		if err != nil {
			return nil, err
		}
		return &RenameProc{Fields: fas}, nil
	case "FuseProc":
		return &FuseProc{}, nil
	case "UniqProc":
		return &UniqProc{}, nil
	case "GroupByProc":
		a, _ := node.Get("keys")
		keys, err := unpackAssignments(a)
		if err != nil {
			return nil, err
		}
		a, _ = node.Get("reducers")
		reducers, err := unpackAssignments(a)
		if err != nil {
			return nil, err
		}
		return &GroupByProc{Reducers: reducers, Keys: keys}, nil
	case "TopProc":
		a, _ := node.Get("fields")
		fields, err := unpackExprs(a)
		if err != nil {
			return nil, err
		}
		return &TopProc{Fields: fields}, nil
	case "PassProc":
		return &PassProc{}, nil
	case "JoinProc":
		n, _ := node.Get("clauses")
		clauses, err := unpackAssignments(n)
		if err != nil {
			return nil, err
		}
		n, err = node.Get("left_key")
		if err != nil {
			return nil, errors.New("ast join proc: missing left_key field")
		}
		leftKey, err := unpackExpression(n)
		if err != nil {
			return nil, err
		}
		n, err = node.Get("right_key")
		if err != nil {
			return nil, errors.New("ast join proc: missing right_key field")
		}
		rightKey, err := unpackExpression(n)
		if err != nil {
			return nil, err
		}
		return &JoinProc{LeftKey: leftKey, RightKey: rightKey, Clauses: clauses}, nil
	default:
		return nil, fmt.Errorf("ast.unpackProc: unknown proc op: %s", op)
	}
}

func unpackExprs(node joe.Interface) ([]Expression, error) {
	if node == nil {
		return nil, nil
	}
	a, ok := node.(joe.Array)
	if !ok {
		return nil, errors.New("ast.unpackExprs: fields property should be an array")
	}
	exprs := make([]Expression, 0, len(a))
	for _, item := range a {
		e, err := unpackExpression(item)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
	}
	return exprs, nil
}

func unpackAssignment(node joe.Interface) (Assignment, error) {
	if node == nil {
		return Assignment{}, errors.New("ast.unpackAssignment: missing assignment field")
	}
	var lhs Expression
	lhsNode, err := node.Get("lhs")
	// LHS is optional as compiler will infer a field name from the LHS.
	if err == nil && lhsNode != nil {
		lhs, err = unpackExpression(lhsNode)
		if err != nil {
			return Assignment{}, err
		}
	}
	rhsNode, err := node.Get("rhs")
	if err != nil {
		return Assignment{}, errors.New("ast.unpackAssignment: missing rhs field")
	}
	rhs, err := unpackExpression(rhsNode)
	if err != nil {
		return Assignment{}, err
	}
	return Assignment{LHS: lhs, RHS: rhs}, nil
}

func unpackAssignments(node joe.Interface) ([]Assignment, error) {
	if node == nil {
		return nil, nil
	}
	a, ok := node.(joe.Array)
	if !ok {
		return nil, errors.New("ast.unpackAssignments: not an array")
	}
	fas := make([]Assignment, 0, len(a))
	for _, item := range a {
		fa, err := unpackAssignment(item)
		if err != nil {
			return nil, err
		}
		fas = append(fas, fa)
	}
	return fas, nil
}

func unpackExpression(node joe.Interface) (Expression, error) {
	if node == nil {
		return nil, errors.New("ast.unpackExpression: no expression provided")
	}
	op, err := getString(node, "op")
	if err != nil {
		return nil, err
	}
	switch op {
	case "UnaryExpr":
		return unpackUnaryExpr(node)
	case "BinaryExpr":
		return unpackBinaryExpr(node)
	case "SelectExpr":
		return unpackSelectExpr(node)
	case "Search":
		return &Search{}, nil
	case "ConditionalExpr":
		conditionNode, err := node.Get("condition")
		if err != nil {
			return nil, errors.New("ConditionalExpr missing condition")
		}
		condition, err := unpackExpression(conditionNode)
		if err != nil {
			return nil, err
		}

		thenNode, err := node.Get("then")
		if err != nil {
			return nil, errors.New("ConditionalExpr missing then")
		}
		thenClause, err := unpackExpression(thenNode)
		if err != nil {
			return nil, err
		}

		elseNode, err := node.Get("else")
		if err != nil {
			return nil, errors.New("ConditionalExpr missing else")
		}
		elseClause, err := unpackExpression(elseNode)
		if err != nil {
			return nil, err
		}
		return &ConditionalExpression{
			Condition: condition,
			Then:      thenClause,
			Else:      elseClause,
		}, nil
	case "FunctionCall":
		args, err := unpackArgs(node)
		if err != nil {
			return nil, err
		}
		return &FunctionCall{Args: args}, nil
	case "CastExpr":
		exprNode, err := node.Get("expr")
		if err != nil {
			return nil, errors.New("CastExpr missing expr")
		}
		expr, err := unpackExpression(exprNode)
		if err != nil {
			return nil, err
		}
		return &CastExpression{Expr: expr}, nil
	case "Reducer":
		exprNode, _ := node.Get("expr")
		var expr Expression
		if exprNode != nil {
			expr, _ = unpackExpression(exprNode)
		}
		whereNode, _ := node.Get("where")
		var where Expression
		if whereNode != nil {
			where, _ = unpackExpression(whereNode)
		}
		return &Reducer{Expr: expr, Where: where}, nil
	case "Literal":
		return &Literal{}, nil
	case "Identifier":
		return &Identifier{}, nil
	case "RootRecord":
		return &RootRecord{}, nil
	case "Empty":
		return &Empty{}, nil
	default:
		return nil, fmt.Errorf("ast.unpackExpression: unknown op %s", op)
	}
}

func unpackArgs(node joe.Interface) ([]Expression, error) {
	argsNode, err := node.Get("args")
	if err != nil {
		return nil, errors.New("ast node missing function args field")
	}
	a, ok := argsNode.(joe.Array)
	if !ok {
		return nil, errors.New("function args property must be an array")
	}
	args := make([]Expression, len(a))
	for i := range args {
		var err error
		args[i], err = unpackExpression(a[i])
		if err != nil {
			return nil, err
		}
	}
	return args, nil
}

func unpackBinaryExpr(node joe.Interface) (*BinaryExpression, error) {
	lhsNode, err := node.Get("lhs")
	if err != nil {
		return nil, errors.New("BinaryExpression missing lhs")
	}
	lhs, err := unpackExpression(lhsNode)
	if err != nil {
		return nil, err
	}
	rhsNode, err := node.Get("rhs")
	if err != nil {
		return nil, errors.New("BinaryExpression missing rhs")
	}
	rhs, err := unpackExpression(rhsNode)
	if err != nil {
		return nil, err
	}
	return &BinaryExpression{LHS: lhs, RHS: rhs}, nil
}

func unpackSelectExpr(node joe.Interface) (*SelectExpression, error) {
	selectorsNode, err := node.Get("selectors")
	if err != nil {
		return nil, errors.New("SelectExpression missing selectors field")
	}
	selectors, err := unpackExprs(selectorsNode)
	if err != nil {
		return nil, err
	}
	return &SelectExpression{Selectors: selectors}, nil
}

func unpackUnaryExpr(node joe.Interface) (*UnaryExpression, error) {
	operandNode, err := node.Get("operand")
	if err != nil {
		return nil, errors.New("UnaryExpr missing operand")
	}
	operand, err := unpackExpression(operandNode)
	if err != nil {
		return nil, err
	}
	return &UnaryExpression{Operand: operand}, nil
}

func UnpackProc(custom Unpacker, m interface{}) (Proc, error) {
	obj := joe.Convert(m)
	proc, err := unpackProc(custom, obj)
	if err != nil {
		return nil, err
	}
	c := &mapstructure.DecoderConfig{
		TagName: "json",
		Result:  proc,
		Squash:  true,
	}
	dec, err := mapstructure.NewDecoder(c)
	if err != nil {
		return nil, err
	}
	return proc, dec.Decode(m)
}

func UnpackExpression(custom Unpacker, m interface{}) (Expression, error) {
	node := joe.Convert(m)
	ex, err := unpackExpression(node)
	if err != nil {
		return nil, err
	}
	c := &mapstructure.DecoderConfig{
		TagName: "json",
		Result:  ex,
		Squash:  true,
	}
	dec, err := mapstructure.NewDecoder(c)
	if err != nil {
		return nil, err
	}
	return ex, dec.Decode(m)
}

func UnpackProgram(custom Unpacker, m interface{}) (*Program, error) {
	node := joe.Convert(m)
	program, err := unpackProgram(node)
	if err != nil {
		return nil, err
	}
	c := &mapstructure.DecoderConfig{
		TagName: "json",
		Result:  program,
		Squash:  true,
	}
	dec, err := mapstructure.NewDecoder(c)
	if err != nil {
		return nil, err
	}
	return program, dec.Decode(m)
}

// UnpackJSON transforms a JSON representation of a proc into an ast.Proc.
func UnpackJSON(custom Unpacker, buf []byte) (Proc, error) {
	if len(buf) == 0 {
		return nil, nil
	}
	obj, err := joe.Unmarshal(buf)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	proc, err := unpackProc(custom, obj)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(buf, proc); err != nil {
		return nil, err
	}
	return proc, nil
}
