package function

import (
	"errors"
	"fmt"

	"github.com/brimsec/zq/anymath"
	"github.com/brimsec/zq/expr/result"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zson"
)

var (
	ErrBadArgument    = errors.New("bad argument")
	ErrNoSuchFunction = errors.New("no such function")
	ErrTooFewArgs     = errors.New("too few arguments")
	ErrTooManyArgs    = errors.New("too many arguments")
)

type Interface interface {
	Call([]zng.Value) (zng.Value, error)
}

var deprecated = map[string]string{
	"Math.abs":              "abs",
	"Math.ceil":             "ceil",
	"Math.floor":            "floor",
	"Math.log":              "log",
	"Math.max":              "max",
	"Math.min":              "min",
	"Math.mod":              "mod",
	"Math.round":            "round",
	"Math.pow":              "pow",
	"Math.sqrt":             "sqrt",
	"String.byteLen":        "len",
	"String.formatFloat":    "type cast, e.g., <float-value>:string",
	"String.formatInt":      "type cast, e.g., <int-value>:string",
	"String.formatIp":       "type cast, e.g., <ip-value>:string",
	"String.parseFloat":     "type cast, e.g., <string-value>:float64",
	"String.parseInt":       "type cast, e.g., <string-value>:int64",
	"String.parseIp":        "type cast, e.g., <string-value>:ip",
	"String.replace":        "replace",
	"String.runeLen":        "rune_len",
	"String.toLower":        "to_lower",
	"String.toUpper":        "to_upper",
	"String.trim":           "trim",
	"Time.fromISO":          "iso",
	"Time.fromMilliseconds": "msec and ype cast, e.g., msec(<msec-value>):time",
	"Time.fromMicroseconds": "usec and type cast, e.g., usec(<usec-value>):time",
	"Time.fromNanoseconds":  "type cast, e.g., <nsec-value>:time",
	"Time.trunc":            "trunc",
	"toBase64":              "to_base64",
	"fromBase64":            "from_base64",
}

func isDeprecated(name string) error {
	msg, ok := deprecated[name]
	if ok {
		return fmt.Errorf("function is deprecated: use %s", msg)
	}
	return nil
}

func New(zctx *resolver.Context, name string, narg int) (Interface, error) {
	if err := isDeprecated(name); err != nil {
		return nil, err
	}
	argmin := 1
	argmax := 1
	var f Interface
	switch name {
	default:
		return nil, ErrNoSuchFunction
	case "len":
		f = &lenFn{}
	case "abs":
		f = &abs{}
	case "ceil":
		f = &ceil{}
	case "floor":
		f = &floor{}
	case "log":
		f = &log{}
	case "max":
		argmax = -1
		f = &reducer{fn: anymath.Max}
	case "min":
		argmax = -1
		f = &reducer{fn: anymath.Min}
	case "mod":
		argmin = 2
		argmax = 2
		f = &mod{}
	case "round":
		f = &round{}
	case "pow":
		argmin = 2
		argmax = 2
		f = &pow{}
	case "sqrt":
		f = &sqrt{}
	case "replace":
		argmin = 3
		argmax = 3
		f = &replace{}
	case "rune_len":
		f = &runeLen{}
	case "to_lower":
		f = &toLower{}
	case "to_upper":
		f = &toUpper{}
	case "trim":
		f = &trim{}
	case "iso":
		f = &iso{}
	case "sec":
		f = &sec{}
	case "msec":
		f = &msec{}
	case "usec":
		f = &usec{}
	case "trunc":
		argmin = 2
		argmax = 2
		f = &trunc{}
	case "typeof":
		f = &typeOf{zson.NewTypeTable(zctx)}
	case "iserr":
		f = &isErr{}
	case "to_base64":
		f = &toBase64{}
	case "from_base64":
		f = &fromBase64{}
	case "network_of":
		argmax = 2
		f = &networkOf{}
	case "reshape":
		argmin = 2
		argmax = 2
		f = &reshape{}
	}
	if argmin != -1 && narg < argmin {
		return nil, ErrTooFewArgs
	}
	if argmax != -1 && narg > argmax {
		return nil, ErrTooManyArgs
	}
	return f, nil
}

func zverr(msg string, err error) (zng.Value, error) {
	return zng.Value{}, fmt.Errorf("%s: %w", msg, err)
}

func badarg(msg string) (zng.Value, error) {
	return zverr(msg, ErrBadArgument)
}

type lenFn struct {
	result.Buffer
}

func (l *lenFn) Call(args []zng.Value) (zng.Value, error) {
	zv := args[0]
	if zv.Bytes == nil {
		return zng.Value{zng.TypeInt64, nil}, nil
	}
	switch zng.AliasedType(args[0].Type).(type) {
	case *zng.TypeArray, *zng.TypeSet:
		len, err := zv.ContainerLength()
		if err != nil {
			return zng.Value{}, err
		}
		return zng.Value{zng.TypeInt64, l.Int(int64(len))}, nil
	case *zng.TypeOfString, *zng.TypeOfBstring, *zng.TypeOfIP, *zng.TypeOfNet:
		v := len(zv.Bytes)
		return zng.Value{zng.TypeInt64, l.Int(int64(v))}, nil
	default:
		return badarg("len")
	}
}

type typeOf struct {
	types *zson.TypeTable
}

func (t *typeOf) Call(args []zng.Value) (zng.Value, error) {
	typ := args[0].Type
	return t.types.LookupValue(typ), nil
}

type isErr struct{}

func (*isErr) Call(args []zng.Value) (zng.Value, error) {
	if args[0].IsError() {
		return zng.True, nil
	}
	return zng.False, nil
}
