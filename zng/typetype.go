package zng

import (
	"fmt"

	"github.com/brimdata/zed/zcode"
)

type TypeOfType struct{}

func (t *TypeOfType) ID() int {
	return IDType
}

func (t *TypeOfType) String() string {
	return "type"
}

func (t *TypeOfType) Marshal(zv zcode.Bytes) (interface{}, error) {
	return t.Format(zv), nil
}

//XXX wrong call FormatType
func (t *TypeOfType) Format(zv zcode.Bytes) string {
	return fmt.Sprintf("(%s)", FormatType(zv))
}
