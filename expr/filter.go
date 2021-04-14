package expr

import (
	"errors"
	"strings"

	"github.com/brimdata/zed/compiler/ast/zed"
	"github.com/brimdata/zed/pkg/byteconv"
	"github.com/brimdata/zed/pkg/stringsearch"
	"github.com/brimdata/zed/zcode"
	"github.com/brimdata/zed/zng"
)

type Filter func(*zng.Record) bool

func LogicalAnd(left, right Filter) Filter {
	return func(p *zng.Record) bool { return left(p) && right(p) }
}

func LogicalOr(left, right Filter) Filter {
	return func(p *zng.Record) bool { return left(p) || right(p) }
}

func LogicalNot(expr Filter) Filter {
	return func(p *zng.Record) bool { return !expr(p) }
}

func Apply(e Evaluator, pred Boolean) Filter {
	return func(r *zng.Record) bool {
		v, err := e.Eval(r)
		if err != nil || v.Type == nil {
			// field (or sub-field) doesn't exist in this record
			return false
		}
		return pred(v)
	}
}

func EvalAny(eval Boolean, recursive bool) Filter {
	if !recursive {
		return func(r *zng.Record) bool {
			it := r.ZvalIter()
			for _, c := range r.Columns() {
				val, _, err := it.Next()
				if err != nil {
					return false
				}
				if eval(zng.Value{c.Type, val}) {
					return true
				}
			}
			return false
		}
	}

	var fn func(v zcode.Bytes, cols []zng.Column) bool
	fn = func(v zcode.Bytes, cols []zng.Column) bool {
		it := v.Iter()
		for _, c := range cols {
			val, _, err := it.Next()
			if err != nil {
				return false
			}
			recType, isRecord := c.Type.(*zng.TypeRecord)
			if isRecord && fn(val, recType.Columns) {
				return true
			} else if !isRecord && eval(zng.Value{c.Type, val}) {
				return true
			}
		}
		return false
	}
	return func(r *zng.Record) bool {
		return fn(r.Bytes, r.Columns())
	}
}

// stringSearch is like strings.Contains() but with case-insensitive
// comparison.
func stringSearch(a, b string) bool {
	alen := len(a)
	blen := len(b)

	if blen > alen {
		return false
	}

	end := alen - blen + 1
	i := 0
	for i < end {
		if strings.EqualFold(a[i:i+blen], b) {
			return true
		}
		i++
	}
	return false
}

var errMatch = errors.New("match")

// SearchRecordOther creates a filter that searches zng records for the
// given value, which must be of a type other than (b)string.  The filter
// matches a record that contains this value either as the value of any
// field or inside any set or array.  It also matches a record if the string
// representaton of the search value appears inside inside any string-valued
// field (or inside any element of a set or array of strings).
func SearchRecordOther(searchtext string, searchval zed.Primitive) (Filter, error) {
	typedCompare, err := Comparison("=", searchval)
	if err != nil {
		return nil, err
	}
	return func(r *zng.Record) bool {
		return errMatch == r.Walk(func(typ zng.Type, body zcode.Bytes) error {
			if zng.IsStringy(typ.ID()) {
				if stringSearch(byteconv.UnsafeString(body), searchtext) {
					return errMatch
				}
			} else if typedCompare(zng.Value{Type: typ, Bytes: body}) {
				return errMatch
			}
			return nil
		})
	}, nil

}

// SearchRecordString handles the special case of string searching -- it
// matches both field names and values.
func SearchRecordString(term string) Filter {
	fieldNameCheck := make(map[zng.Type]bool)
	var nameIter stringsearch.FieldNameIter
	return func(r *zng.Record) bool {
		// Memoize the result of a search across the names in the
		// record columns for each unique record type.
		match, ok := fieldNameCheck[r.Type]
		if !ok {
			nameIter.Init(zng.TypeRecordOf(r.Type))
			for !nameIter.Done() {
				if stringSearch(byteconv.UnsafeString(nameIter.Next()), term) {
					match = true
					break
				}
			}
			fieldNameCheck[r.Type] = match
		}
		if match {
			return true
		}
		return errMatch == r.Walk(func(typ zng.Type, body zcode.Bytes) error {
			if zng.IsStringy(typ.ID()) &&
				stringSearch(byteconv.UnsafeString(body), term) {
				return errMatch
			}
			return nil
		})
	}
}

type FilterEvaluator Filter

func (f FilterEvaluator) Eval(rec *zng.Record) (zng.Value, error) {
	if f(rec) {
		return zng.True, nil
	}
	return zng.False, nil
}
