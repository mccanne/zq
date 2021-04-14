package zed

type Value interface {
	valueNode()
}

type ImpliedValue struct {
	Kind string `json:"kind" unpack:""`
	Of   Any    `json:"of"`
}

type DefValue struct {
	Kind     string `json:"kind" unpack:""`
	Of       Any    `json:"of"`
	TypeName string `json:"type_name"`
}

type CastValue struct {
	Kind string `json:"kind" unpack:""`
	Of   Value  `json:"of"`
	Type Type   `json:"type"`
}

func (*ImpliedValue) valueNode() {}
func (*DefValue) valueNode()     {}
func (*CastValue) valueNode()    {}

type Any interface {
	anyNode()
}

type (
	Primitive struct {
		Kind string `json:"kind" unpack:""`
		Type string `json:"type"`
		Text string `json:"text"`
	}
	Record struct {
		Kind   string  `json:"Kind" unpack:""`
		Fields []Field `json:"fields"`
	}
	Field struct {
		Name  string `json:"name"`
		Value Value  `json:"value"`
	}
	Array struct {
		Kind     string  `json:"kind" unpack:""`
		Elements []Value `json:"elements"`
	}
	Set struct {
		Kind     string  `json:"kind" unpack:""`
		Elements []Value `json:"elements"`
	}
	Enum struct {
		Kind string `json:"kind" unpack:""`
		Name string `json:"name"`
	}
	Map struct {
		Kind    string  `json:"kind" unpack:""`
		Entries []Entry `json:"entries"`
	}
	Entry struct {
		Key   Value `json:"key"`
		Value Value `json:"value"`
	}
	TypeValue struct {
		Kind  string `json:"kind" unpack:""`
		Value Type   `json:"value"`
	}
)

func (*Primitive) anyNode() {}
func (*Record) anyNode()    {}
func (*Array) anyNode()     {}
func (*Set) anyNode()       {}
func (*Enum) anyNode()      {}
func (*Map) anyNode()       {}
func (*TypeValue) anyNode() {}

func (*Primitive) ExprAST() {}
func (*TypeValue) ExprAST() {}

func (*Primitive) ExprDAG() {}
func (*TypeValue) ExprDAG() {}
