package dag

// This module is derived from the GO ast design pattern
// https://golang.org/pkg/go/ast/
//
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

import (
	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/field"
)

type Op interface {
	opNode()
}

// Ops

type (
	Cut struct {
		Kind string       `json:"kind" unpack:""`
		Args []Assignment `json:"args"`
	}
	Drop struct {
		Kind string `json:"kind" unpack:""`
		Args []Expr `json:"args"`
	}
	Filter struct {
		Kind string `json:"kind" unpack:""`
		Expr Expr   `json:"expr"`
	}
	Fuse struct {
		Kind string `json:"kind" unpack:""`
	}
	Head struct {
		Kind  string `json:"kind" unpack:""`
		Count int    `json:"count"`
	}
	Join struct {
		Kind     string       `json:"kind" unpack:""`
		Style    string       `json:"style"`
		LeftKey  Expr         `json:"left_key"`
		RightKey Expr         `json:"right_key"`
		Args     []Assignment `json:"args"`
	}
	Parallel struct {
		Kind string `json:"kind" unpack:""`
		Ops  []Op   `json:"ops"`
		// XXX move mergeby to a downstream proc.  the optimizatoin
		// can bookkeep this info outside of the dag.
		MergeBy      field.Static `json:"merge_by,omitempty"`
		MergeReverse bool         `json:"merge_reverse,omitempty"`
	}
	Pass struct {
		Kind string `json:"kind" unpack:""`
	}
	Pick struct {
		Kind string       `json:"kind" unpack:""`
		Args []Assignment `json:"args"`
	}
	Put struct {
		Kind string       `json:"kind" unpack:""`
		Args []Assignment `json:"args"`
	}
	Rename struct {
		Kind string       `json:"kind" unpack:""`
		Args []Assignment `json:"args"`
	}
	Sequential struct {
		Kind string `json:"kind" unpack:""`
		Ops  []Op   `json:"ops"`
	}
	Shape struct {
		Kind string `json:"kind" unpack:""`
	}
	Sort struct {
		Kind       string `json:"kind" unpack:""`
		Args       []Expr `json:"args"`
		SortDir    int    `json:"sortdir"`
		NullsFirst bool   `json:"nullsfirst"`
	}
	Summarize struct {
		Kind         string         `json:"kind" unpack:""`
		Duration     *zed.Primitive `json:"duration"`
		Limit        int            `json:"limit"`
		Keys         []Assignment   `json:"keys"`
		Aggs         []Assignment   `json:"aggs"`
		InputSortDir int            `json:"input_sort_dir,omitempty"`
		PartialsIn   bool           `json:"partials_in,omitempty"`
		PartialsOut  bool           `json:"partials_out,omitempty"`
	}
	Switch struct {
		Kind  string `json:"kind" unpack:""`
		Cases []Case `json:"cases"`
		// XXX move mergeby to a downstream proc.  the optimizatoin
		// can bookkeep this info outside of the dag.
		MergeBy      field.Static `json:"merge_by,omitempty"`
		MergeReverse bool         `json:"merge_reverse,omitempty"`
	}
	Tail struct {
		Kind  string `json:"kind" unpack:""`
		Count int    `json:"count"`
	}
	Top struct {
		Kind  string `json:"kind" unpack:""`
		Limit int    `json:"limit"`
		Args  []Expr `json:"args"`
		Flush bool   `json:"flush"`
	}
	Uniq struct {
		Kind  string `json:"kind" unpack:""`
		Cflag bool   `json:"cflag"`
	}
)

// Various Op fields

type (
	Assignment struct {
		Kind string `json:"kind" unpack:""`
		LHS  Expr   `json:"lhs"`
		RHS  Expr   `json:"rhs"`
	}
	Agg struct {
		Kind  string `json:"kind" unpack:""`
		Name  string `json:"name"`
		Expr  Expr   `json:"expr"`
		Where Expr   `json:"where"`
	}
	Case struct {
		Expr Expr `json:"expr"`
		Op   Op   `json:"op"`
	}
	Method struct {
		Name string `json:"name"`
		Args []Expr `json:"args"`
	}
)

func (*Sequential) opNode()   {}
func (*Parallel) opNode()     {}
func (*Switch) opNode()       {}
func (*Sort) opNode()         {}
func (*Cut) opNode()          {}
func (*Pick) opNode()         {}
func (*Drop) opNode()         {}
func (*Head) opNode()         {}
func (*Tail) opNode()         {}
func (*Pass) opNode()         {}
func (*Filter) opNode()       {}
func (*Uniq) opNode()         {}
func (*Summarize) opNode()    {}
func (*Top) opNode()          {}
func (*Put) opNode()          {}
func (*Rename) opNode()       {}
func (*Fuse) opNode()         {}
func (*Join) opNode()         {}
func (*Const) opNode()        {}
func (*TypeProc) opNode()     {}
func (*Shape) opNode()        {}
func (*FieldCutter) opNode()  {}
func (*TypeSplitter) opNode() {}

func FanIn(op Op) int {
	switch op := op.(type) {
	case *Sequential:
		return FanIn(op.Ops[0])
	case *Join:
		return 2
	}
	return 1
}

func FilterToOp(e Expr) *Filter {
	return &Filter{
		Kind: "Filter",
		Expr: e,
	}
}

func (p *Path) String() string {
	return field.Static(p.Name).String()
}

// === THESE SHOULD BE RENAMED AND MADE PART OF THE LANGUAGE ===

type FieldCutter struct {
	Kind  string       `json:"kind" unpack:""`
	Field field.Static `json:"field"`
	Out   field.Static `json:"out"`
}

type TypeSplitter struct {
	Kind     string       `json:"kind" unpack:""`
	Key      field.Static `json:"key"`
	TypeName string       `json:"type_name"`
}

// === THESE WILL BE DEPRECATED ===

type Const struct {
	Kind string `json:"kind" unpack:""`
	Name string `json:"name"`
	Expr Expr   `json:"expr"`
}
type TypeProc struct {
	Kind string   `json:"kind" unpack:""`
	Name string   `json:"name"`
	Type zed.Type `json:"type"`
}
