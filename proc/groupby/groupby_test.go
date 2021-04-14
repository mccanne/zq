package groupby_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/brimdata/zed/api"
	"github.com/brimdata/zed/compiler"
	"github.com/brimdata/zed/compiler/ast"
	"github.com/brimdata/zed/driver"
	"github.com/brimdata/zed/pkg/nano"
	"github.com/brimdata/zed/pkg/test"
	"github.com/brimdata/zed/proc/groupby"
	"github.com/brimdata/zed/zbuf"
	"github.com/brimdata/zed/zio/zsonio"
	"github.com/brimdata/zed/zng"
	"github.com/brimdata/zed/zson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Data sets for tests:
const in = `
{key1:"a",key2:"x",n:1 (int32)} (=0)
{key1:"a",key2:"y",n:2} (0)
{key1:"b",key2:"z",n:1} (0)
`

const groupSingleOut = `
{key1:"a",count:2 (uint64)} (=0)
{key1:"b",count:1} (0)
`

const groupMultiOut = `
{key1:"a",key2:"x",count:1 (uint64)} (=0)
{key1:"a",key2:"y",count:1} (0)
{key1:"b",key2:"z",count:1} (0)
`

const unsetKeyIn = `
{key1:null (string),key2:null (string),n:3 (int32)} (=0)
{key1:null,key2:null,n:4} (0)
`

const groupSingleOut_unsetOut = `
{key1:"a",count:2 (uint64)} (=0)
{key1:"b",count:1} (0)
{key1:null,count:2} (0)
`

const missingField = `
{key3:"a",n:1 (int32)} (=0)
{key3:"b",n:2} (0)
`

const differentTypeIn = `
{key1:10.0.0.1,n:1 (int32)} (=0)
{key1:10.0.0.2,n:1} (0)
{key1:10.0.0.1,n:1} (0)
`

const differentTypeOut = `
{key1:10.0.0.1,count:2 (uint64)} (=0)
{key1:10.0.0.2,count:1} (0)
{key1:"a",count:2 (uint64)} (=1)
{key1:"b",count:1} (1)
`

const reducersOut = `
{key1:"a",any:1 (int32),sum:3,avg:1.5,min:1,max:2} (=0)
{key1:"b",any:1,sum:1,avg:1.,min:1,max:1} (0)
`

const arrayKeyIn = `
{arr:null (0=([int32])),val:2 (int32)} (=1)
{arr:[1,2],val:2} (1)
{arr:[1,2],val:3} (1)
`

const arrayKeyOut = `
{arr:null (0=([int32])),count:1 (uint64)} (=1)
{arr:[1,2],count:2} (1)
`

const nestedKeyIn = `
{rec:{i:1 (int32),s:"bleah"} (=0),val:1} (=1)
{rec:{i:1,s:"bleah"},val:2} (1)
{rec:{i:2,s:"bleah"},val:3} (1)
`

const nestedKeyOut = `
{rec:{i:1 (int32)} (=0),count:2 (uint64)} (=1)
{rec:{i:2},count:1} (1)
`
const nestedKeyAssignedOut = `
{newkey:1 (int32),count:2 (uint64)} (=0)
{newkey:2,count:1} (0)
`

const unsetIn = `
{key:"key1",val:5}
{key:"key2",val:null (int64)}
`

const unsetOut = `
{key:"key1",sum:5}
{key:"key2",sum:null (int64)}
`

const notPresentIn = `
{key:"key1"}
`

const notPresentOut = `
{key:"key1",max:null (null)} (=0)
`

const mixedIn = `
{key:"k",f:5 (int32)} (=0)
{key:"k",f:"bleah"}
`

const mixedOut = `
{key:"k",first:5 (int32),last:"bleah"} (=0)
`

const aliasIn = `
{host:127.0.0.1 (=ipaddr)} (=0)
{host:127.0.0.2}
`

const aliasOut = `
{host:127.0.0.1 (=ipaddr),count:1 (uint64)} (=0)
{host:127.0.0.2,count:1 (uint64)} (=1)
`

const computedKeyIn = `
{s:"foo",i:2 (uint64),j:2 (uint64)} (=0)
{s:"FOO",i:2,j:2} (0)
`

const computedKeyOut = `
{s:"foo",ij:4 (uint64),count:2 (uint64)} (=0)
`

//XXX this should go in a shared package
type suite []test.Internal

func (s suite) runSystem(t *testing.T) {
	for _, d := range s {
		t.Run(d.Name, func(t *testing.T) {
			results, err := d.Run()
			require.NoError(t, err)
			assert.Exactly(t, d.Expected, results, "Wrong query results...\nQuery: %s\nInput: %s\n", d.Query, d.Input)
		})
	}
}

func (s *suite) add(t test.Internal) {
	*s = append(*s, t)
}

func New(name, input, output, cmd string) test.Internal {
	output = strings.ReplaceAll(output, "\n\n", "\n")
	return test.Internal{
		Name:         name,
		Query:        "* | " + cmd,
		Input:        input,
		OutputFormat: "zson",
		Expected:     test.Trim(output),
	}
}

func tests() suite {
	s := suite{}

	// Test a simple groupby
	s.add(New("simple", in, groupSingleOut, "count() by key1 | sort key1"))
	s.add(New("simple-assign", in, groupSingleOut, "count() by key1=key1 | sort key1"))

	// Test that unset key values work correctly
	s.add(New("unset-keys", in+unsetKeyIn, groupSingleOut_unsetOut, "count() by key1 | sort key1"))
	s.add(New("unset-keys-at-start", unsetKeyIn+in, groupSingleOut_unsetOut, "count() by key1 | sort key1"))

	// Test grouping by multiple fields
	s.add(New("multiple-fields", in, groupMultiOut, "count() by key1,key2 | sort key1, key2"))

	// Test that records missing groupby fields are ignored
	s.add(New("missing-fields", in+missingField, groupSingleOut, "count() by key1 | sort key1"))

	// Test that input with different key types works correctly
	s.add(New("different-key-types", in+differentTypeIn, differentTypeOut, "count() by key1 | sort key1"))

	// Test various reducers
	s.add(New("reducers", in, reducersOut, "any(n), sum(n), avg(n), min(n), max(n) by key1 | sort key1"))

	// Check out of bounds array indexes
	s.add(New("array-out-of-bounds", arrayKeyIn, arrayKeyOut, "count() by arr | sort"))

	// Check groupby key inside a record
	s.add(New("key-in-record", nestedKeyIn, nestedKeyOut, "count() by rec.i | sort rec.i"))

	// Test reducers with unset inputs
	s.add(New("unset-inputs", unsetIn, unsetOut, "sum(val) by key | sort"))

	// Test reducers with missing operands
	s.add(New("not-present", notPresentIn, notPresentOut, "max(val) by key | sort"))

	s.add(New("aliases", aliasIn, aliasOut, "count() by host | sort host"))

	// Tests with assignments and computed keys
	s.add(New("unset-keys-computed", in+unsetKeyIn, groupSingleOut_unsetOut, "count() by key1=to_lower(to_upper(key1)) | sort key1"))
	s.add(New("unset-keys-assign", in+unsetKeyIn, strings.ReplaceAll(groupSingleOut_unsetOut, "key1", "newkey"), "count() by newkey=key1 | sort newkey"))
	s.add(New("unset-keys-at-start-assign", unsetKeyIn+in, strings.ReplaceAll(groupSingleOut_unsetOut, "key1", "newkey"), "count() by newkey=key1 | sort newkey"))
	s.add(New("multiple-fields-assign", in, strings.ReplaceAll(groupMultiOut, "key2", "newkey"), "count() by key1,newkey=key2 | sort key1, newkey"))
	s.add(New("key-in-record-assign", nestedKeyIn, nestedKeyAssignedOut, "count() by newkey=rec.i | sort newkey"))
	s.add(New("computed-key", computedKeyIn, computedKeyOut, "count() by s=to_lower(s), ij=i+j | sort"))
	return s
}

func TestGroupbySystem(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		tests().runSystem(t)
	})
	t.Run("spill", func(t *testing.T) {
		saved := groupby.DefaultLimit
		groupby.DefaultLimit = 1
		defer func() {
			groupby.DefaultLimit = saved
		}()
		tests().runSystem(t)
	})
}

func compileGroupBy(code string) (*ast.Summarize, error) {
	parsed, err := compiler.ParseProc(code)
	if err != nil {
		return nil, err
	}
	sp := parsed.(*ast.Sequential)
	return sp.Procs[0].(*ast.Summarize), nil
}

type countReader struct {
	mu sync.Mutex
	n  int
	r  zbuf.Reader
}

func (cr *countReader) records() int {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return cr.n
}

func (cr *countReader) Read() (*zng.Record, error) {
	rec, err := cr.r.Read()
	if rec != nil {
		cr.mu.Lock()
		cr.n++
		cr.mu.Unlock()
	}
	return rec, err
}

type testGroupByDriver struct {
	n      int
	writer zbuf.Writer
	cb     func(n int)
}

func (d *testGroupByDriver) Write(cid int, batch zbuf.Batch) error {
	for _, r := range batch.Records() {
		d.n++
		if err := d.writer.Write(r); err != nil {
			return err
		}
	}
	d.cb(d.n)
	return nil
}

func (d *testGroupByDriver) Warn(msg string) error {
	panic("shouldn't warn")
}

func (d *testGroupByDriver) ChannelEnd(int) error         { return nil }
func (d *testGroupByDriver) Stats(api.ScannerStats) error { return nil }

func TestGroupbyStreamingSpill(t *testing.T) {

	// This test verifies that with sorted input, spillable groupby streams results as input arrives.
	//
	// The sorted input key is ts. The input and config parameters are carefully chosen such that:
	// - spills are not aligned with ts changes (at least some
	//   transitions from ts=n to ts=n+1 happen mid-spill)
	// - secondary keys repeat in a ts bin
	//
	// Together these conditions test that the read barrier (using
	// GroupByAggregator.maxSpillKey) does not read a key from a
	// spill before that all records for that key have been
	// written to the spill.
	//
	savedBatchSize := zbuf.ScannerBatchSize
	zbuf.ScannerBatchSize = 1
	savedBatchSizeGroupByLimit := groupby.DefaultLimit
	groupby.DefaultLimit = 2
	defer func() {
		zbuf.ScannerBatchSize = savedBatchSize
		groupby.DefaultLimit = savedBatchSizeGroupByLimit
	}()

	const totRecs = 200
	const recsPerTs = 9
	const uniqueIpsPerTs = 3

	var data []string
	for i := 0; i < totRecs; i++ {
		t := i / recsPerTs
		data = append(data, fmt.Sprintf("{ts:%s,ip:1.1.1.%d}", nano.Unix(int64(t), 0), i%uniqueIpsPerTs))
	}

	runOne := func(inputSortKey string) []string {
		proc, err := compiler.ParseProc("every 1s count() by ip")
		assert.NoError(t, err)

		zctx := zson.NewContext()
		zr := zson.NewReader(strings.NewReader(strings.Join(data, "\n")), zctx)
		cr := &countReader{r: zr}
		var outbuf bytes.Buffer
		zw := zsonio.NewWriter(&nopCloser{&outbuf}, zsonio.WriterOpts{})
		d := &testGroupByDriver{
			writer: zw,
			cb: func(n int) {
				if inputSortKey != "" {
					if n == uniqueIpsPerTs {
						require.Less(t, cr.records(), totRecs)
					}
				}
			},
		}
		err = driver.Run(context.Background(), d, proc, zctx, cr, driver.Config{
			ReaderSortKey: inputSortKey,
		})
		require.NoError(t, err)
		outData := strings.Split(outbuf.String(), "\n")
		sort.Strings(outData)
		return outData
	}

	res := runOne("") // run once in non-streaming mode to have reference results to compare with.
	resStreaming := runOne("ts")
	require.Equal(t, res, resStreaming)
}

type nopCloser struct{ io.Writer }

func (*nopCloser) Close() error { return nil }
