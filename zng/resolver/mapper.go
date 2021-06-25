package resolver

import (
	"sync"

	"github.com/brimdata/zed"
	"github.com/brimdata/zed/zson"
)

type Mapper struct {
	outputCtx *zson.Context
	mu        sync.RWMutex
	types     []zed.Type
}

func NewMapper(out *zson.Context) *Mapper {
	return &Mapper{outputCtx: out}
}

// Lookup tranlates Zed types by type ID from one context to another.
// The first context is implied by the argument to Lookup() and the output
// type context is explicitly determined by the argument to NewMapper().
// If a binding has not yet been entered, nil is returned and Enter()
// should be called to create the binding.  There is a race here when two
// threads attempt to update the same ID, but it is safe because the
// outputContext will return the same the pointer so the second update
// does not change anything.
func (m *Mapper) Lookup(td int) zed.Type {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if td < len(m.types) {
		return m.types[td]
	}
	return nil
}

func (m *Mapper) Enter(id int, ext zed.Type) (zed.Type, error) {
	typ, err := m.outputCtx.TranslateType(ext)
	if err != nil {
		return nil, err
	}
	if typ != nil {
		m.EnterType(id, typ)
		return typ, nil
	}
	return nil, nil
}

func (m *Mapper) EnterType(td int, typ zed.Type) {
	m.mu.Lock()
	if td >= cap(m.types) {
		new := make([]zed.Type, td+1, td*2)
		copy(new, m.types)
		m.types = new
	} else if td >= len(m.types) {
		m.types = m.types[:td+1]
	}
	m.types[td] = typ
	m.mu.Unlock()
}
