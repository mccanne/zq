package search

import (
	"fmt"
	"net/http"

	"github.com/brimsec/zq/pkg/bufwriter"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zio"
	"github.com/brimsec/zq/zio/ndjsonio"
)

// NDJSONOutput implements the Output inteface and writes NDJSON encoded-output
// directly to the client as text/NDJSON.
type NDJSONOutput struct {
	response http.ResponseWriter
	writer   *ndjsonio.Writer
	buffer   *bufwriter.Writer
}

func NewNDJSONOutput(response http.ResponseWriter) *NDJSONOutput {
	o := &NDJSONOutput{
		response: response,
		writer:   ndjsonio.NewWriter(zio.NopCloser(response)),
	}
	return o
}

func (n *NDJSONOutput) flush() {
	n.response.(http.Flusher).Flush()
}

func (*NDJSONOutput) Collect() interface{} {
	return "TBD" //XXX
}

func (n *NDJSONOutput) SendBatch(cid int, batch zbuf.Batch) error {
	for _, rec := range batch.Records() {
		if err := n.writer.Write(rec); err != nil {
			// Embed an error in the NDJSON output.  We can't report
			// an http error because we already started successfully
			// streaming records.
			msg := fmt.Sprintf("query error: %s\n", err)
			n.response.Write([]byte(msg))
			return err
		}
	}
	batch.Unref()
	n.flush()
	return nil
}

func (n *NDJSONOutput) End(ctrl interface{}) error {
	if err := n.writer.Close(); err != nil {
		return err
	}
	return nil
}

func (r *NDJSONOutput) SendControl(ctrl interface{}) error {
	return nil
}

func (r *NDJSONOutput) ContentType() string {
	return MimeTypeNDJSON
}
