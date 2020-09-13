package zcode

import (
	"fmt"
)

// Iter iterates over the sequence of values encoded in Bytes.
type Iter Bytes

// Done returns true if no values remain.
func (i *Iter) Done() bool {
	return len(*i) == 0
}

// Next returns the body of the next value along with a boolean that is true if
// the value is a container.  It returns an empty slice for an empty or
// zero-length value and nil for an unset value.
func (i *Iter) Next() (Bytes, bool, error) {
	// The tag is zero for an unset value; otherwise, it is the value's
	// length plus one.
	u64, n := uvarint(*i)
	if n <= 0 {
		return nil, false, fmt.Errorf("bad uvarint: %d", n)
	}
	if tagIsUnset(u64) {
		*i = (*i)[n:]
		return nil, tagIsContainer(u64), nil
	}
	end := n + tagLength(u64)
	val := (*i)[n:end]
	*i = (*i)[end:]
	return Bytes(val), tagIsContainer(u64), nil
}

// NextTagAndBody returns the next value as a slice containing the value's
// undecoded tag followed by its body along with a boolean that is true if the
// value is a container.
func (i *Iter) NextTagAndBody() (Bytes, bool, error) {
	u64, n := uvarint(*i)
	if n <= 0 {
		return nil, false, fmt.Errorf("bad uvarint: %d", n)
	}
	if !tagIsUnset(u64) {
		n += tagLength(u64)
	}
	val := (*i)[:n]
	*i = (*i)[n:]
	return Bytes(val), tagIsContainer(u64), nil
}

// BodyFromTagAndBody returns the body of the Bytes value returned by NextTagAndBody.
func BodyFromTagAndBody(tagbody Bytes) (Bytes, error) {
	p := Iter(tagbody)
	body, _, err := p.Next()
	return body, err
}
