package expr

import (
	"fmt"

	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
)

// A ShaperTransform represents one of the different transforms that a
// shaper can apply.  The transforms are represented as a bit flags that
// can be bitwise-ored together to create a single shaping operator that
// represents the composition of all operators.  This composition is efficient
// as it is carried once per incoming type signature and then the resulting
// operator is run for every value of that type.
type ShaperTransform int

const (
	Cast ShaperTransform = 1 << iota
	Fill
	Crop
	Order
)

type op int

const (
	copyPrimitive op = iota // copy field fromIndex from input record
	copyContainer
	castPrimitive // cast field fromIndex from input record
	null          // write null
	array         // build array
	set           // build set
	record        // build record
)

// A step is a recursive data structure encoding a series of
// copy/cast steps to be carried out over an input record.
type step struct {
	op        op
	fromIndex int
	castTypes struct{ from, to zng.Type } // for op == castPrimitive
	// if op == record, contains one op for each column.
	// if op == array, contains one op for all array elements.
	children []step
}

func (s *step) append(step step) {
	s.children = append(s.children, step)
}

// create the step needed to build a record of type out from a
// record of type in. The two types must be compatible, meaning that
// the input type must be an unordered subset of the input type
// (where 'unordered' means that if the output type has record fields
// [a b] and the input type has fields [b a] that is ok). It is also
// ok for leaf primitive types to be different; if they are a casting
// step is inserted.
func createStepRecord(in, out *zng.TypeRecord) (step, error) {
	s := step{op: record}
	for _, outCol := range out.Columns {
		ind, ok := in.ColumnOfField(outCol.Name)
		if !ok {
			s.append(step{op: null})
			continue
		}
		inCol := in.Columns[ind]
		child, err := createStep(inCol.Type, outCol.Type)
		if err != nil {
			return step{}, err
		}
		child.fromIndex = ind
		s.append(child)
	}
	return s, nil
}

func createStepArray(in, out zng.Type) (step, error) {
	s := step{op: array}
	innerOp, err := createStep(in, out)
	if err != nil {
		return step{}, nil
	}
	s.append(innerOp)
	return s, nil
}

func createStepSet(in, out zng.Type) (step, error) {
	s := step{op: set}
	innerOp, err := createStep(in, out)
	if err != nil {
		return step{}, nil
	}
	s.append(innerOp)
	return s, nil
}

func isCollectionType(t zng.Type) bool {
	switch zng.AliasOf(t).(type) {
	case *zng.TypeArray, *zng.TypeSet:
		return true
	}
	return false
}

func createStep(in, out zng.Type) (step, error) {
	switch {
	case in.ID() == out.ID():
		if zng.IsContainerType(in) {
			return step{op: copyContainer}, nil
		} else {
			return step{op: copyPrimitive}, nil
		}
	case zng.IsRecordType(in) && zng.IsRecordType(out):
		return createStepRecord(in.(*zng.TypeRecord), out.(*zng.TypeRecord))
	case zng.IsPrimitiveType(in) && zng.IsPrimitiveType(out):
		return step{op: castPrimitive, castTypes: struct{ from, to zng.Type }{in, out}}, nil
	case isCollectionType(in):
		if _, ok := out.(*zng.TypeArray); ok {
			return createStepArray(zng.InnerType(in), zng.InnerType(out))
		}
		if _, ok := out.(*zng.TypeSet); ok {
			return createStepSet(zng.InnerType(in), zng.InnerType(out))
		}
		fallthrough
	default:
		return step{}, fmt.Errorf("createStep incompatible column types %s and %s\n", in, out)
	}
}

func (s *step) castPrimitive(in zcode.Bytes, b *zcode.Builder) *zng.Value {
	if in == nil {
		b.AppendNull()
		return nil
	}
	toType := zng.AliasOf(s.castTypes.to)
	pc := LookupPrimitiveCaster(toType)
	v, err := pc(zng.Value{s.castTypes.from, in})
	if err != nil {
		b.AppendNull()
		return nil
	}
	if v.Type != toType {
		// v isn't the "to" type, so we can't safely append v.Bytes to
		// the builder. See https://github.com/brimdata/zed/issues/2710.
		if v.Type == zng.TypeError {
			return &v
		}
		panic(fmt.Sprintf("expr: got %T from primitive caster, expected %T", v.Type, toType))
	}
	b.AppendPrimitive(v.Bytes)
	return nil
}

func (s *step) build(in zcode.Bytes, b *zcode.Builder) *zng.Value {
	switch s.op {
	case copyPrimitive:
		b.AppendPrimitive(in)
	case copyContainer:
		b.AppendContainer(in)
	case castPrimitive:
		if zerr := s.castPrimitive(in, b); zerr != nil {
			return zerr
		}
	case record:
		if in == nil {
			b.AppendNull()
			return nil
		}
		b.BeginContainer()
		if zerr := s.buildRecord(in, b); zerr != nil {
			return zerr
		}
		b.EndContainer()
	case array, set:
		if in == nil {
			b.AppendNull()
			return nil
		}
		b.BeginContainer()
		iter := in.Iter()
		for !iter.Done() {
			zv, _, err := iter.Next()
			if err != nil {
				panic(err)
			}
			if zerr := s.children[0].build(zv, b); zerr != nil {
				return zerr
			}
		}
		if s.op == set {
			b.TransformContainer(zng.NormalizeSet)
		}
		b.EndContainer()
	}
	return nil
}

func (s *step) buildRecord(in zcode.Bytes, b *zcode.Builder) *zng.Value {
	for _, step := range s.children {
		switch step.op {
		case null:
			b.AppendNull()
			continue
		}
		// Using getNthFromContainer means we iterate from the
		// beginning of the record for each field. An
		// optimization (for shapes that don't require field
		// reordering) would be make direct use of a
		// zcode.Iter along with keeping track of our
		// position.
		bytes, err := getNthFromContainer(in, uint(step.fromIndex))
		if err != nil {
			panic(err)
		}
		if zerr := step.build(bytes, b); zerr != nil {
			return zerr
		}
	}
	return nil
}

// A shaper is a per-input type ID "spec" that contains the output
// type and the op to create an output record.
type shaper struct {
	typ  zng.Type
	step step
}

type ConstShaper struct {
	zctx       *zson.Context
	b          zcode.Builder
	expr       Evaluator
	shapeTo    zng.Type
	shapers    map[int]*shaper // map from input type ID to shaper
	transforms ShaperTransform
}

// NewConstShaper returns a shaper that will shape the result of expr
// to the provided shapeTo type.
func NewConstShaper(zctx *zson.Context, expr Evaluator, shapeTo zng.Type, tf ShaperTransform) *ConstShaper {
	return &ConstShaper{
		zctx:       zctx,
		expr:       expr,
		shapeTo:    shapeTo,
		shapers:    make(map[int]*shaper),
		transforms: tf,
	}
}

type Shaper struct {
	zctx       *zson.Context
	typExpr    Evaluator
	expr       Evaluator
	shapers    map[zng.Type]*ConstShaper
	transforms ShaperTransform
}

// NewShaper returns a shaper that will shape the result of expr
// to the type returned by typExpr.
func NewShaper(zctx *zson.Context, expr, typExpr Evaluator, tf ShaperTransform) *Shaper {
	return &Shaper{
		zctx:       zctx,
		typExpr:    typExpr,
		expr:       expr,
		shapers:    make(map[zng.Type]*ConstShaper),
		transforms: tf,
	}
}

func (s *Shaper) Eval(rec *zng.Record) (zng.Value, error) {
	typVal, err := s.typExpr.Eval(rec)
	if err != nil {
		return zng.Value{}, err
	}
	if typVal.Type != zng.TypeType {
		return zng.NewErrorf("shaper function type argument is not a type"), nil
	}
	shapeTo, err := s.zctx.LookupByValue(typVal.Bytes)
	if err != nil {
		return zng.NewErrorf("shaper encountered unknown type value: %s", err), nil
	}
	shaper, ok := s.shapers[shapeTo]
	if !ok {
		if zng.TypeRecordOf(shapeTo) == nil {
			return zng.NewErrorf("shaper function type argument is not a record type: %q", shapeTo), nil
		}
		shaper = NewConstShaper(s.zctx, s.expr, shapeTo, s.transforms)
		s.shapers[shapeTo] = shaper
	}
	return shaper.Eval(rec)
}

func (s *ConstShaper) Apply(in *zng.Record) (*zng.Record, error) {
	v, err := s.Eval(in)
	if err != nil {
		return nil, err
	}
	return zng.NewRecord(v.Type.(*zng.TypeRecord), v.Bytes), nil
}

func (c *ConstShaper) Eval(in *zng.Record) (zng.Value, error) {
	inVal, err := c.expr.Eval(in)
	if err != nil {
		return zng.Value{}, err
	}
	inType, ok := inVal.Type.(*zng.TypeRecord)
	if !ok {
		return inVal, nil
	}
	id := in.Type.ID()
	s, ok := c.shapers[id]
	if !ok {
		s, err = createShaper(c.zctx, c.transforms, c.shapeTo, inType)
		if err != nil {
			return zng.Value{}, err
		}
		c.shapers[id] = s
	}
	if s.typ.ID() == id {
		return zng.Value{s.typ, inVal.Bytes}, nil
	}
	c.b.Reset()
	if zerr := s.step.buildRecord(inVal.Bytes, &c.b); zerr != nil {
		typ, err := c.zctx.LookupTypeRecord([]zng.Column{{Name: "error", Type: zerr.Type}})
		if err != nil {
			return zng.Value{}, err
		}
		c.b.AppendPrimitive(zerr.Bytes)
		return zng.Value{typ, c.b.Bytes()}, nil
	}
	return zng.Value{s.typ, c.b.Bytes()}, nil
}

func createShaper(zctx *zson.Context, transforms ShaperTransform, shapeTo zng.Type, inType *zng.TypeRecord) (*shaper, error) {
	var err error
	spec := zng.TypeRecordOf(shapeTo)
	typ := inType
	if transforms&Cast > 0 {
		typ, err = castRecordType(zctx, typ, spec)
		if err != nil {
			return nil, err
		}
	}
	if transforms&Crop > 0 {
		typ, err = cropRecordType(zctx, typ, spec)
		if err != nil {
			return nil, err
		}
	}
	if transforms&Fill > 0 {
		typ, err = fillRecordType(zctx, typ, spec)
		if err != nil {
			return nil, err
		}
	}
	if transforms&Order > 0 {
		typ, err = orderRecordType(zctx, typ, spec)
		if err != nil {
			return nil, err
		}
	}
	step, err := createStepRecord(inType, typ)
	var final zng.Type
	if typ.ID() == shapeTo.ID() {
		// If the underlying records are the same, then use the
		// spec record as it might be an alias and the intention
		// would be to cast to the named type.
		final = shapeTo
	} else {
		final = typ
	}
	return &shaper{final, step}, err
}

// cropRecordType applies a crop (as specified by the record type 'spec')
// to a record type and returns the resulting record type.
func cropRecordType(zctx *zson.Context, input, spec *zng.TypeRecord) (*zng.TypeRecord, error) {
	cols := make([]zng.Column, 0)
	for _, inCol := range input.Columns {
		ind, ok := spec.ColumnOfField(inCol.Name)
		if !ok {
			// 1. Field not in crop: drop.
			continue
		}
		inType := zng.AliasOf(inCol.Type)
		specCol := spec.Columns[ind]
		specType := zng.AliasOf(specCol.Type)
		switch {
		case zng.IsPrimitiveType(inType):
			// 2. Field is non-record in input: keep (regardless of crop record-ness)
			cols = append(cols, inCol)
		case zng.IsRecordType(inType) && zng.IsRecordType(specType):
			// 3. Both records: recurse
			out, err := cropRecordType(zctx, inType.(*zng.TypeRecord), specType.(*zng.TypeRecord))
			if err != nil {
				return nil, err
			}
			cols = append(cols, zng.Column{inCol.Name, out})
		case isCollectionType(inType) && isCollectionType(specType):
			inInner := zng.AliasOf(zng.InnerType(inType))
			specInner := zng.AliasOf(zng.InnerType(specType))
			if zng.IsRecordType(inInner) && zng.IsRecordType(specInner) {
				// 4. array/set of records
				inner, err := cropRecordType(zctx, inInner.(*zng.TypeRecord), specInner.(*zng.TypeRecord))
				if err != nil {
					return nil, err
				}
				var t zng.Type
				if _, ok := inCol.Type.(*zng.TypeArray); ok {
					t, err = zctx.LookupTypeArray(inner), nil
				} else {
					t, err = zctx.LookupTypeSet(inner), nil
				}
				if err != nil {
					return nil, err
				}
				cols = append(cols, zng.Column{inCol.Name, t})
			} else {
				cols = append(cols, inCol)
			}
		default:
			// 5. container input but non-container in crop: keep crop
			cols = append(cols, specCol)

		}
	}
	return zctx.LookupTypeRecord(cols)
}

// orderRecordType applies a field order (as specified by the record type 'spec')
// to a record type and returns the resulting record type.
func orderRecordType(zctx *zson.Context, input, spec *zng.TypeRecord) (*zng.TypeRecord, error) {
	cols := make([]zng.Column, 0)
	// Simple order algorithm creates a list with all specified
	// 'order' fields present in input, followed by any other
	// fields that were in input but not specified in order. Two
	// examples:
	//
	// 1. (a b c d) order (c e b a) -> (c b a d)
	//
	// 2. (a b c d) order (d e c b) -> (d c b a)
	//
	// The second example with 'a' moving to the back suggests
	// that we may want to use a different algorithm where
	// unspecified fields "stay where they were". Specifically we
	// might prefer that the result be (a d c b). We will learn
	// with use, so starting with simpler algorithm for now.
	//
	for _, specCol := range spec.Columns {
		if ind, ok := input.ColumnOfField(specCol.Name); ok {
			inCol := input.Columns[ind]
			inType := zng.AliasOf(inCol.Type)
			specType := zng.AliasOf(specCol.Type)
			if zng.IsRecordType(inType) && zng.IsRecordType(specType) {
				if nested, err := orderRecordType(zctx, inType.(*zng.TypeRecord), specType.(*zng.TypeRecord)); err != nil {
					return nil, err
				} else {
					cols = append(cols, zng.Column{specCol.Name, nested})
				}
				continue
			}
			if isCollectionType(inCol.Type) && isCollectionType(specCol.Type) &&
				zng.IsRecordType(zng.InnerType(inCol.Type)) &&
				zng.IsRecordType(zng.InnerType(specCol.Type)) {
				inInner := zng.AliasOf(zng.InnerType(inCol.Type))
				specInner := zng.AliasOf(zng.InnerType(specCol.Type))
				if inner, err := orderRecordType(zctx, inInner.(*zng.TypeRecord), specInner.(*zng.TypeRecord)); err != nil {
					return nil, err
				} else {
					var err error
					var t zng.Type
					if _, ok := inCol.Type.(*zng.TypeArray); ok {
						t, err = zctx.LookupTypeArray(inner), nil
					} else {
						t, err = zctx.LookupTypeSet(inner), nil
					}
					if err != nil {
						return nil, err
					}
					cols = append(cols, zng.Column{specCol.Name, t})
				}
				continue
			}
			cols = append(cols, inCol)
		}
	}
	for _, inCol := range input.Columns {
		if !spec.HasField(inCol.Name) {
			cols = append(cols, inCol)
		}
	}
	return zctx.LookupTypeRecord(cols)
}

// fillRecordType applies a fill (as specified by the record type 'spec')
// to a record type and returns the resulting record type.
func fillRecordType(zctx *zson.Context, input, spec *zng.TypeRecord) (*zng.TypeRecord, error) {
	cols := make([]zng.Column, len(input.Columns), len(input.Columns)+len(spec.Columns))
	copy(cols, input.Columns)
	for _, specCol := range spec.Columns {
		if i, ok := input.ColumnOfField(specCol.Name); ok {
			specType := zng.AliasOf(specCol.Type)
			inCol := input.Columns[i]
			inType := zng.AliasOf(inCol.Type)
			// Field is present both in input and spec: recurse if
			// both records, or select appropriate type if not.
			if specRecType, ok := specType.(*zng.TypeRecord); ok {
				if inRecType, ok := inType.(*zng.TypeRecord); ok {
					filled, err := fillRecordType(zctx, inRecType, specRecType)
					if err != nil {
						return nil, err
					}
					cols[i] = zng.Column{specCol.Name, filled}
				} else {
					cols[i] = specCol
				}
				continue
			}
			if isCollectionType(inType) && isCollectionType(specType) {
				inInner := zng.AliasOf(zng.InnerType(inCol.Type))
				specInner := zng.AliasOf(zng.InnerType(specCol.Type))
				if zng.IsRecordType(inInner) && zng.IsRecordType(specInner) {
					if inner, err := fillRecordType(zctx, inInner.(*zng.TypeRecord), specInner.(*zng.TypeRecord)); err != nil {
						return nil, err
					} else {
						var err error
						var t zng.Type
						if _, ok := inCol.Type.(*zng.TypeArray); ok {
							t, err = zctx.LookupTypeArray(inner), nil
						} else {
							t, err = zctx.LookupTypeSet(inner), nil
						}
						if err != nil {
							return nil, err
						}
						cols[i] = zng.Column{specCol.Name, t}
					}
				}
			}
		} else {
			cols = append(cols, specCol)
		}
	}
	return zctx.LookupTypeRecord(cols)
}

// castRecordType applies a cast (as specified by the record type 'spec')
// to a record type and returns the resulting record type.
func castRecordType(zctx *zson.Context, input, spec *zng.TypeRecord) (*zng.TypeRecord, error) {
	cols := make([]zng.Column, 0)
	for _, inCol := range input.Columns {
		// For each input column, check if we have a matching
		// name in the cast spec.
		ind, ok := spec.ColumnOfField(inCol.Name)
		if !ok {
			// 1. No match: output type unmodified.
			cols = append(cols, inCol)
			continue
		}
		specCol := spec.Columns[ind]
		if _, ok := specCol.Type.(*zng.TypeMap); ok {
			return nil, fmt.Errorf("cannot yet use maps in shaping functions")
		}
		if inCol.Type.ID() == specCol.Type.ID() {
			// Field has same type in cast: output type unmodified.
			cols = append(cols, specCol)
			continue
		}
		castType, err := castType(zctx, inCol.Type, specCol.Type)
		if err != nil {
			return nil, err
		}
		cols = append(cols, zng.Column{inCol.Name, castType})
	}
	return zctx.LookupTypeRecord(cols)
}

func castType(zctx *zson.Context, inType, specType zng.Type) (zng.Type, error) {
	switch {
	case zng.IsRecordType(inType) && zng.IsRecordType(specType):
		// Matching field is a record: recurse.
		inRec := zng.AliasOf(inType).(*zng.TypeRecord)
		castRec := zng.AliasOf(specType).(*zng.TypeRecord)
		return castRecordType(zctx, inRec, castRec)
	case zng.IsPrimitiveType(inType) && zng.IsPrimitiveType(specType):
		// Matching field is a primitive: output type is cast type.
		if LookupPrimitiveCaster(zng.AliasOf(specType)) == nil {
			return nil, fmt.Errorf("cast to %s not implemented", specType)
		}
		return specType, nil
	case isCollectionType(inType) && isCollectionType(specType):
		out, err := castType(zctx, zng.InnerType(inType), zng.InnerType(specType))
		if err != nil {
			return nil, err
		}
		if _, ok := zng.AliasOf(specType).(*zng.TypeArray); ok {
			return zctx.LookupTypeArray(out), nil
		}
		return zctx.LookupTypeSet(out), nil
	default:
		// Non-castable type pair with at least one
		// (non-record) container: output column is left
		// unchanged.
		return inType, nil
	}
}
