package zed

type Type interface {
	typeNode()
}

type (
	TypePrimitive struct {
		Kind string `json:"kind" unpack:""`
		Name string `json:"name"`
	}
	TypeRecord struct {
		Kind   string      `json:"kind" unpack:""`
		Fields []TypeField `json:"fields"`
	}
	TypeField struct {
		Name string `json:"name"`
		Type Type   `json:"type"`
	}
	TypeArray struct {
		Kind string `json:"kind" unpack:""`
		Type Type   `json:"type"`
	}
	TypeSet struct {
		Kind string `json:"kind" unpack:""`
		Type Type   `json:"type"`
	}
	TypeUnion struct {
		Kind  string `json:"kind" unpack:""`
		Types []Type `json:"types"`
	}
	// Enum has just the elements and relies on the semantic checker
	// to determine a type from the decorator either within or from above.
	TypeEnum struct {
		Kind     string  `json:"kind" unpack:""`
		Elements []Field `json:"elements"`
	}
	TypeMap struct {
		Kind    string `json:"kind" unpack:""`
		KeyType Type   `json:"key_type"`
		ValType Type   `json:"val_type"`
	}
	TypeNull struct {
		Kind string `json:"kind" unpack:""`
	}
	TypeName struct {
		Kind string `json:"kind" unpack:""`
		Name string `json:"name"`
	}
	TypeDef struct {
		Kind string `json:"kind" unpack:""`
		Name string `json:"name"`
		Type Type   `json:"type"`
	}
)

func (*TypePrimitive) typeNode() {}
func (*TypeRecord) typeNode()    {}
func (*TypeArray) typeNode()     {}
func (*TypeSet) typeNode()       {}
func (*TypeUnion) typeNode()     {}
func (*TypeEnum) typeNode()      {}
func (*TypeMap) typeNode()       {}
func (*TypeNull) typeNode()      {}
func (*TypeName) typeNode()      {}
func (*TypeDef) typeNode()       {}
