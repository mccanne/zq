package index

import (
	"fmt"

	"github.com/brimdata/zed/compiler"
	"github.com/brimdata/zed/field"
	"github.com/brimdata/zed/zio/tzngio"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"github.com/segmentio/ksuid"
)

type IndexKind string

const (
	IndexType  IndexKind = "type"
	IndexField IndexKind = "field"
	IndexZed   IndexKind = "zed"
)

// Index contains the runtime configuration for an index.
type Index struct {
	Framesize int         `zng:"framesize,omitempty"`
	ID        ksuid.KSUID `zng:"id"`
	Name      string      `zng:"name"`
	Keys      field.List  `zng:"keys,omitempty"`
	Kind      IndexKind   `zng:"kind"`
	Value     string      `zng:"type"`
}

func ParseIndex(pattern string) (Index, error) {
	if pattern[0] == ':' {
		typ, err := zson.NewContext().LookupByName(pattern[1:])
		if err != nil {
			return Index{}, err
		}
		return NewTypeIndex(typ), nil
	}
	return NewFieldIndex(pattern), nil
}

func NewTypeIndex(typ zng.Type) Index {
	return Index{
		ID:    ksuid.New(),
		Kind:  IndexType,
		Value: tzngio.TypeString(typ),
	}
}

// NewFieldIndex creates an index that will index the field passed in as argument.
// It is currently an error to try to index a field name that appears as different types.
func NewFieldIndex(fieldName string) Index {
	return Index{
		ID:    ksuid.New(),
		Kind:  IndexField,
		Value: fieldName,
	}
}

func NewZedIndex(prog, name string, keys field.List) (Index, error) {
	// make sure it compiles
	if _, err := compiler.ParseProc(prog); err != nil {
		return Index{}, err
	}
	return Index{
		ID:    ksuid.New(),
		Keys:  keys,
		Kind:  IndexZed,
		Name:  name,
		Value: prog,
	}, nil
}

// Equivalent determine if the provided Index is equivalent to the receiver. It
// should used to check if a Definition already contains and equivalent index.
func (i Index) Equivalent(r2 Index) bool {
	if i.Kind != r2.Kind || i.Value != r2.Value {
		return false
	}
	if i.Kind == IndexZed {
		return i.Name == r2.Name
	}
	return true
}

const keyName = "key"

func (i Index) zedQuery() (string, error) {
	switch i.Kind {
	case IndexType:
		return fmt.Sprintf("explode this by %s as %s | count() by %s | sort %s", i.Value, keyName, keyName, keyName), nil
	case IndexField:
		return fmt.Sprintf("cut %s:=%s | count() by %s | sort %s", keyName, i.Value, keyName, keyName), nil
	case IndexZed:
		return i.Value, nil
	default:
		return "", fmt.Errorf("unknown index kind: %s", i.Kind)
	}
}

func (i Index) String() string {
	name := i.Value
	if i.Kind == IndexZed {
		name = i.Name
	}
	return fmt.Sprintf("%s->%s", i.Kind, name)
}

type Indices []Index

func (indices Indices) Lookup(id ksuid.KSUID) *Index {
	if i := indices.indexOf(id); i >= 0 {
		return &indices[i]
	}
	return nil
}

// Add checks if Indices already has an equivalent Index and if it does not
// returns Indices with the Index appended to it. Returns a non-nil Index pointer
// if an equivalent Index is found.
func (indices Indices) Add(index Index) (Indices, *Index) {
	for _, i := range indices {
		if i.Equivalent(index) {
			return indices, &i
		}
	}
	return append(indices, index), nil
}

// LookupDelete checks the Indices list for a index matching the provided ID
// returning the deleted index if found.
func (indices Indices) LookupDelete(id ksuid.KSUID) (Indices, *Index) {
	if i := indices.indexOf(id); i >= 0 {
		index := indices[i]
		return append(indices[:i], indices[i+1:]...), &index
	}
	return indices, nil
}

func (indices Indices) indexOf(id ksuid.KSUID) int {
	for i, index := range indices {
		if index.ID == id {
			return i
		}
	}
	return -1
}

func (indices Indices) IDs() []ksuid.KSUID {
	ids := make([]ksuid.KSUID, len(indices))
	for k, index := range indices {
		ids[k] = index.ID
	}
	return ids
}
