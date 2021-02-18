package microindex_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brimsec/zq/compiler"
	"github.com/brimsec/zq/driver"
	"github.com/brimsec/zq/microindex"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio/tzngio"
	"github.com/brimsec/zq/zng"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearch(t *testing.T) {
	const data = `
#0:record[key:string,value:string]
0:[key1;value1;]
0:[key2;value2;]
0:[key3;value3;]
0:[key4;value4;]
0:[key5;value5;]
0:[key6;value6;]`
	finder := buildAndOpen(t, reader(data))
	keyRec, err := finder.ParseKeys("key2")
	require.NoError(t, err)
	rec, err := finder.Lookup(keyRec)
	require.NoError(t, err)
	require.NotNil(t, rec)
	value, err := rec.Slice(1)
	require.NoError(t, err)
	value2 := zng.EncodeString("value2")
	assert.Equal(t, value, value2, "key lookup failed")
}

func TestMicroIndex(t *testing.T) {
	const N = 5
	dir := tempDir(t)
	path := filepath.Join(dir, "test2.zng")
	stream, err := newReader(N)
	require.NoError(t, err)
	zctx := resolver.NewContext()
	writer, err := microindex.NewWriter(zctx, path)
	require.NoError(t, err)
	err = zbuf.Copy(writer, stream)
	require.NoError(t, err)
	err = writer.Close()
	require.NoError(t, err)
	reader, err := microindex.NewReader(zctx, path)
	require.NoError(t, err)
	defer reader.Close() //nolint:errcheck
	r, err := reader.NewSectionReader(0)
	require.NoError(t, err)
	n := 0
	for {
		rec, err := r.Read()
		if rec == nil {
			break
		}
		require.NoError(t, err)
		n++
	}
	assert.Exactly(t, N, n, "number of pairs read from microindex file doesn't match number written")
}

func TestCompare(t *testing.T) {
	const records = `
#0:record[ts:int64,offset:int64]
0:[20;10;]
0:[18;9;]
0:[16;8;]
0:[14;7;]
0:[12;6;]
0:[10;5;]
0:[8;4;]
0:[6;3;]
0:[4;2;]
0:[2;1;]
`
	type testcase struct {
		value         int64
		gte, lte, eql int64
	}
	cases := []testcase{
		{9, 10, 8, -1},
		{1, 2, -1, -1},
		{22, -1, 20, -1},
		{12, 12, 12, 12},
	}
	runtest := func(t *testing.T, finder *microindex.Finder, op string, value int64, expected int64) {
		t.Run(fmt.Sprintf("%d%s%d", expected, op, value), func(t *testing.T) {
			k, err := finder.ParseKeys(fmt.Sprintf("%d", value))
			require.NoError(t, err)

			var rec *zng.Record
			switch op {
			case ">=":
				rec, err = finder.ClosestGTE(k)
			case "<=":
				rec, err = finder.ClosestLTE(k)
			case "==":
				rec, err = finder.Lookup(k)
			}

			require.NoError(t, err)
			if expected == -1 {
				assert.Nil(t, rec)
			} else {
				require.NotNil(t, rec)
				v, err := rec.AccessInt("ts")
				require.NoError(t, err)
				assert.Equal(t, expected, v)
			}
		})

	}
	desc := buildAndOpen(t, reader(records), microindex.Keys("ts"), microindex.Order(zbuf.OrderDesc))
	t.Run("Descending", func(t *testing.T) {
		for _, c := range cases {
			runtest(t, desc, ">=", c.value, c.gte)
			runtest(t, desc, "<=", c.value, c.lte)
			runtest(t, desc, "==", c.value, c.eql)
		}
	})
	r, err := driver.NewReader(context.Background(), compiler.MustParseProgram("sort ts"), resolver.NewContext(), reader(records))
	require.NoError(t, err)
	asc := buildAndOpen(t, r, microindex.Keys("ts"), microindex.Order(zbuf.OrderAsc))
	t.Run("Ascending", func(t *testing.T) {
		for _, c := range cases {
			runtest(t, asc, ">=", c.value, c.gte)
			runtest(t, asc, "<=", c.value, c.lte)
			runtest(t, asc, "==", c.value, c.eql)
		}
	})
}

func buildAndOpen(t *testing.T, r zbuf.Reader, opts ...microindex.Option) *microindex.Finder {
	return openFinder(t, build(t, r, opts...))
}

func openFinder(t *testing.T, path string) *microindex.Finder {
	uri, err := iosrc.ParseURI(path)
	require.NoError(t, err)
	zctx := resolver.NewContext()
	finder, err := microindex.NewFinder(context.Background(), zctx, uri)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, finder.Close()) })
	return finder
}

func build(t *testing.T, r zbuf.Reader, opts ...microindex.Option) string {
	path := filepath.Join(tempDir(t), "test.zng")
	writer, err := microindex.NewWriter(resolver.NewContext(), path, opts...)
	require.NoError(t, err)
	require.NoError(t, zbuf.Copy(writer, r))
	require.NoError(t, writer.Close())
	return path
}

func reader(logs string) *tzngio.Reader {
	return tzngio.NewReader(strings.NewReader(logs), resolver.NewContext())
}

func newReader(size int) (*tzngio.Reader, error) {
	var lines []string
	lines = append(lines, "#0:record[key:string,value:int32]")
	for i := 0; i < size; i++ {
		line := fmt.Sprintf("0:[port:port:%d;%d;]", i, i)
		lines = append(lines, line)
	}
	return reader(strings.Join(lines, "\n")), nil
}

func tempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

/* not yet
func BenchmarkWrite(b *testing.B) {
	stream := newEntryStream(5 << 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dir, err := ioutil.TempDir("", "table_test")
		if err != nil {
			b.Error(err)
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "table.zng")
		if err := table.BuildTable(path, stream); err != nil {
			b.Error(err)
		}
		// tab, err := table.OpenTable(path)
		// if err != nil {
		// b.Error(err)
		// }
		// fmt.Println("table size: ", tab.Size())
	}
}

func BenchmarkRead(b *testing.B) {
	dir, err := ioutil.TempDir("", "table_test")
	if err != nil {
		b.Error(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "table.zng")
	stream := newEntryStream(5 << 20)
	if err := table.BuildTable(path, stream); err != nil {
		b.Error(err)
	}
	tab, err := table.OpenTable(path)
	if err != nil {
		b.Error(err)
	}
	// fmt.Println("table size: ", tab.Size())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		itr := tab.Iterator()
		for key, _ := itr.Next(); key != ""; key, _ = itr.Next() {
		}
	}
}
*/
