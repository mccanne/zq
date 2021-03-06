package dag

import (
	"errors"

	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/pkg/unpack"
)

var unpacker = unpack.New(
	zed.Array{},
	ArrayExpr{},
	Assignment{},
	BinaryExpr{},
	Call{},
	Cast{},
	zed.CastValue{},
	Conditional{},
	Const{},
	Cut{},
	zed.DefValue{},
	Dot{},
	Drop{},
	Explode{},
	zed.Enum{},
	File{},
	Filter{},
	From{},
	Fuse{},
	Summarize{},
	Head{},
	HTTP{},
	zed.ImpliedValue{},
	Join{},
	zed.Map{},
	MapExpr{},
	Shape{},
	Parallel{},
	Pass{},
	Path{},
	Pick{},
	Pool{},
	zed.Primitive{},
	Put{},
	zed.Record{},
	Agg{},
	RegexpMatch{},
	RegexpSearch{},
	RecordExpr{},
	Ref{},
	Rename{},
	Search{},
	SelectExpr{},
	SeqExpr{},
	Sequential{},
	zed.Set{},
	SetExpr{},
	Sort{},
	Switch{},
	Tail{},
	Top{},
	Trunk{},
	zed.TypeArray{},
	zed.TypeDef{},
	zed.TypeEnum{},
	zed.TypeMap{},
	zed.TypeName{},
	zed.TypeNull{},
	zed.TypePrimitive{},
	TypeProc{},
	zed.TypeRecord{},
	zed.TypeSet{},
	zed.TypeUnion{},
	zed.TypeValue{},
	UnaryExpr{},
	Uniq{},
)

func UnpackJSON(buf []byte) (interface{}, error) {
	if len(buf) == 0 {
		return nil, nil
	}
	return unpacker.Unmarshal(buf)
}

// UnpackJSONAsOp transforms a JSON representation of an operator into an dag.Op.
func UnpackJSONAsOp(buf []byte) (Op, error) {
	result, err := UnpackJSON(buf)
	if result == nil || err != nil {
		return nil, err
	}
	op, ok := result.(Op)
	if !ok {
		return nil, errors.New("JSON object is not a DAG operator")
	}
	return op, nil
}

func UnpackMapAsOp(m interface{}) (Op, error) {
	object, err := unpacker.UnmarshalObject(m)
	if object == nil || err != nil {
		return nil, err
	}
	op, ok := object.(Op)
	if !ok {
		return nil, errors.New("dag.UnpackMapAsOp: not an Op")
	}
	return op, nil
}

func UnpackMapAsExpr(m interface{}) (Expr, error) {
	object, err := unpacker.UnmarshalObject(m)
	if object == nil || err != nil {
		return nil, err
	}
	e, ok := object.(Expr)
	if !ok {
		return nil, errors.New("dag.UnpackMapAsExpr: not an Expr")
	}
	return e, nil
}
