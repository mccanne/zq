package zio

import (
	"github.com/brimdata/zed"
	"github.com/brimdata/zed/zng/resolver"
	"github.com/brimdata/zed/zson"
)

type Mapper struct {
	Reader
	mapper *resolver.Mapper
}

func NewMapper(reader Reader, zctx *zson.Context) *Mapper {
	return &Mapper{
		Reader: reader,
		mapper: resolver.NewMapper(zctx),
	}
}

func (m *Mapper) Read() (*zed.Record, error) {
	rec, err := m.Reader.Read()
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	id := zed.TypeID(rec.Type)
	sharedType := m.mapper.Lookup(id)
	if sharedType == nil {
		sharedType, err = m.mapper.Enter(id, rec.Type)
		if err != nil {
			return nil, err
		}
	}
	rec.Type = sharedType
	return rec, nil
}
