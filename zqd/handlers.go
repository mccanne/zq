package zqd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/brimsec/zq/pcap"
	"github.com/brimsec/zq/pkg/ctxio"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zng/resolver"
	"github.com/brimsec/zq/zqd/api"
	"github.com/brimsec/zq/zqd/ingest"
	"github.com/brimsec/zq/zqd/search"
	"github.com/brimsec/zq/zqd/space"
	"github.com/brimsec/zq/zqe"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func errorResponse(e error) (status int, ae *api.Error) {
	status = http.StatusInternalServerError
	ae = &api.Error{Type: "Error"}

	var ze *zqe.Error
	if !errors.As(e, &ze) {
		ae.Message = e.Error()
		return
	}

	switch ze.Kind {
	case zqe.Invalid:
		status = http.StatusBadRequest
	case zqe.NotFound:
		status = http.StatusNotFound
	case zqe.Exists:
		status = http.StatusBadRequest
	case zqe.Conflict:
		status = http.StatusConflict
	}

	ae.Kind = ze.Kind.String()
	ae.Message = ze.Message()
	return
}

func respond(c *Core, w http.ResponseWriter, r *http.Request, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		c.requestLogger(r).Warn("Error writing response", zap.Error(err))
	}
}

func respondError(c *Core, w http.ResponseWriter, r *http.Request, err error) {
	status, ae := errorResponse(err)
	if status >= 500 {
		c.requestLogger(r).Warn("error", zap.Int("status", status), zap.Error(err))
	}
	respond(c, w, r, status, ae)
}

func request(c *Core, w http.ResponseWriter, r *http.Request, apiobj interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(apiobj); err != nil {
		respondError(c, w, r, zqe.E(zqe.Invalid, err))
		return false
	}
	return true
}

func handleSearch(c *Core, w http.ResponseWriter, r *http.Request) {
	var req api.SearchRequest
	if !request(c, w, r, &req) {
		return
	}

	s, err := c.spaces.Get(req.Space)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	srch, err := search.NewSearchOp(req)
	if err != nil {
		// XXX This always returns bad request but should return status codes
		// that reflect the nature of the returned error.
		respondError(c, w, r, err)
		return
	}

	out, err := getSearchOutput(w, r)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	w.Header().Set("Content-Type", out.ContentType())
	if err := srch.Run(ctx, s.Storage(), out); err != nil {
		c.requestLogger(r).Warn("Error writing response", zap.Error(err))
	}
}

func getSearchOutput(w http.ResponseWriter, r *http.Request) (search.Output, error) {
	ctrl := true
	if r.URL.Query().Get("noctrl") != "" {
		ctrl = false
	}
	format := r.URL.Query().Get("format")
	switch format {
	case "csv":
		return search.NewCSVOutput(w, ctrl), nil
	case "ndjson":
		return search.NewNDJSONOutput(w), nil
	case "zjson":
		return search.NewJSONOutput(w, search.DefaultMTU, ctrl), nil
	case "zng":
		return search.NewZngOutput(w, ctrl), nil
	default:
		return nil, zqe.E(zqe.Invalid, "unsupported search format: %s", format)
	}
}

func handlePcapSearch(c *Core, w http.ResponseWriter, r *http.Request) {
	s := extractSpace(c, w, r)
	if s == nil {
		return
	}

	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	var req api.PcapSearch
	if err := req.FromQuery(r.URL.Query()); err != nil {
		respondError(c, w, r, zqe.E(zqe.Invalid, err))
		return
	}
	pspace, ok := s.(space.PcapSpace)
	if !ok {
		respondError(c, w, r, zqe.E(zqe.Invalid, "space does not support pcap searches"))
		return
	}
	pcapstore := pspace.PcapStore()
	if pcapstore.Empty() {
		respondError(c, w, r, zqe.E(zqe.NotFound, "no pcap in this space"))
		return
	}
	reader, err := pcapstore.NewSearch(ctx, req)
	if err == pcap.ErrNoPcapsFound {
		respondError(c, w, r, zqe.E(zqe.NotFound, err))
		return
	}
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/vnd.tcpdump.pcap")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%s.pcap", reader.ID()))
	_, err = ctxio.Copy(ctx, w, reader)
	if err != nil {
		c.requestLogger(r).Error("Error writing packet response", zap.Error(err))
	}
}

func handleSpaceList(c *Core, w http.ResponseWriter, r *http.Request) {
	spaces, err := c.spaces.List(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	respond(c, w, r, http.StatusOK, spaces)
}

func handleSpaceGet(c *Core, w http.ResponseWriter, r *http.Request) {
	s := extractSpace(c, w, r)
	if s == nil {
		return
	}

	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	info, err := s.Info(ctx)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	respond(c, w, r, http.StatusOK, info)
}

func handleSpacePost(c *Core, w http.ResponseWriter, r *http.Request) {
	var req api.SpacePostRequest
	if !request(c, w, r, &req) {
		return
	}

	sp, err := c.spaces.Create(req)
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	info, err := sp.Info(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	respond(c, w, r, http.StatusOK, info)
}

func handleSubspacePost(c *Core, w http.ResponseWriter, r *http.Request) {
	s := extractSpace(c, w, r)
	if s == nil {
		return
	}

	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	var req api.SubspacePostRequest
	if !request(c, w, r, &req) {
		return
	}

	sp, err := c.spaces.CreateSubspace(s, req)
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	info, err := sp.Info(ctx)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	respond(c, w, r, http.StatusOK, info)
}

func handleSpacePut(c *Core, w http.ResponseWriter, r *http.Request) {
	s := extractSpace(c, w, r)
	if s == nil {
		return
	}

	_, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	var req api.SpacePutRequest
	if !request(c, w, r, &req) {
		return
	}
	if err := c.spaces.UpdateSpace(s, req); err != nil {
		respondError(c, w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleSpaceDelete(c *Core, w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	id, ok := v["space"]
	if !ok {
		respondError(c, w, r, zqe.E(zqe.Invalid, "no space id in path"))
		return
	}

	if err := c.spaces.Delete(api.SpaceID(id)); err != nil {
		respondError(c, w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handlePcapPost(c *Core, w http.ResponseWriter, r *http.Request) {
	if !c.HasZeek() {
		respondError(c, w, r, zqe.E(zqe.Invalid, "pcap post not supported: Zeek not found"))
		return
	}
	logger := c.requestLogger(r)

	s := extractSpace(c, w, r)
	if s == nil {
		return
	}

	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	var req api.PcapPostRequest
	if !request(c, w, r, &req) {
		return
	}

	pspace, ok := s.(space.PcapSpace)
	if !ok {
		respondError(c, w, r, zqe.E(zqe.Invalid, "space does not support pcap import"))
		return
	}
	pcapstore := pspace.PcapStore()
	logstore, ok := s.Storage().(ingest.ClearableStore)
	if !ok {
		respondError(c, w, r, zqe.E(zqe.Invalid, "storage does not support pcap import"))
		return
	}
	op, warnings, err := ingest.NewPcapOp(ctx, pcapstore, logstore, req.Path, c.ZeekLauncher)
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/ndjson")
	w.WriteHeader(http.StatusAccepted)
	pipe := api.NewJSONPipe(w)
	taskID := c.getTaskID()
	taskStart := api.TaskStart{Type: "TaskStart", TaskID: taskID}
	if err := pipe.Send(taskStart); err != nil {
		logger.Warn("Error sending payload", zap.Error(err))
		return
	}
	for _, warning := range warnings {
		err := pipe.Send(api.LogPostWarning{
			Type:    "PcapPostWarning",
			Warning: warning,
		})
		if err != nil {
			logger.Warn("error sending payload", zap.Error(err))
			return
		}
	}
	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()
	for {
		var done bool
		select {
		case <-op.Done():
			done = true
		case <-op.Snap():
		case <-ticker.C:
		}

		sum, err := s.Storage().Summary(ctx)
		if err != nil {
			logger.Warn("Error reading storage summary", zap.Error(err))
			return
		}

		status := api.PcapPostStatus{
			Type:          "PcapPostStatus",
			StartTime:     op.StartTime,
			UpdateTime:    nano.Now(),
			PcapSize:      op.PcapSize,
			PcapReadSize:  op.PcapReadSize(),
			SnapshotCount: op.SnapshotCount(),
			Span:          &sum.Span,
		}
		if err := pipe.Send(status); err != nil {
			logger.Warn("Error sending payload", zap.Error(err))
			return
		}
		if done {
			break
		}
	}
	taskEnd := api.TaskEnd{Type: "TaskEnd", TaskID: taskID}

	if ctx.Err() != nil {
		taskEnd.Error = &api.Error{Type: "Error", Message: "pcap post operation canceled"}
	} else if operr := op.Err(); operr != nil {
		var ok bool
		taskEnd.Error, ok = operr.(*api.Error)
		if !ok {
			taskEnd.Error = &api.Error{Type: "Error", Message: operr.Error()}
		}
	}
	if err := pipe.SendFinal(taskEnd); err != nil {
		logger.Warn("Error sending payload", zap.Error(err))
		return
	}
}

func handleLogPost(c *Core, w http.ResponseWriter, r *http.Request) {
	s := extractSpace(c, w, r)
	if s == nil {
		return
	}
	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	var req api.LogPostRequest
	if !request(c, w, r, &req) {
		return
	}
	if len(req.Paths) == 0 {
		respondError(c, w, r, zqe.E(zqe.Invalid, "empty paths"))
		return
	}
	op, err := ingest.NewLogOp(ctx, s.Storage(), req)
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/ndjson")
	w.WriteHeader(http.StatusAccepted)
	logger := c.requestLogger(r)
	pipe := api.NewJSONPipe(w)
	if err := pipe.SendStart(0); err != nil {
		logger.Warn("error sending payload", zap.Error(err))
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
loop:
	for {
		select {
		case warning, ok := <-op.Status():
			if !ok {
				break loop
			}
			err := pipe.Send(api.LogPostWarning{
				Type:    "LogPostWarning",
				Warning: warning,
			})
			if err != nil {
				logger.Warn("error sending payload", zap.Error(err))
				return
			}
		case <-ticker.C:
			if err := pipe.Send(op.Stats()); err != nil {
				logger.Warn("error sending payload", zap.Error(err))
				return
			}
		}
	}
	// send final status
	if err := pipe.Send(op.Stats()); err != nil {
		logger.Warn("error sending payload", zap.Error(err))
		return
	}
	if err := pipe.SendEnd(0, op.Error()); err != nil {
		logger.Warn("error sending payload", zap.Error(err))
		return
	}
}

func handleIndexSearch(c *Core, w http.ResponseWriter, r *http.Request) {
	s := extractSpace(c, w, r)
	if s == nil {
		return
	}
	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	var req api.IndexSearchRequest
	if !request(c, w, r, &req) {
		return
	}

	store, ok := s.Storage().(search.IndexSearcher)
	if !ok {
		respondError(c, w, r, zqe.E(zqe.Invalid, "space storage does not support index search"))
		return
	}
	srch, err := search.NewIndexSearchOp(ctx, store, req)
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer srch.Close()

	out, err := getSearchOutput(w, r)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	w.Header().Set("Content-Type", out.ContentType())
	if err := srch.Run(out); err != nil {
		c.requestLogger(r).Warn("Error writing response", zap.Error(err))
	}
}

type ArchiveStater interface {
	ArchiveStat(context.Context, *resolver.Context) (zbuf.ReadCloser, error)
}

func handleArchiveStat(c *Core, w http.ResponseWriter, r *http.Request) {
	s := extractSpace(c, w, r)
	if s == nil {
		return
	}
	ctx, cancel, err := s.StartOp(r.Context())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer cancel()

	store, ok := s.Storage().(ArchiveStater)
	if !ok {
		respondError(c, w, r, zqe.E(zqe.Invalid, "space storage does not support archive stat"))
		return
	}
	rc, err := store.ArchiveStat(ctx, resolver.NewContext())
	if err != nil {
		respondError(c, w, r, err)
		return
	}
	defer rc.Close()

	out, err := getSearchOutput(w, r)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	w.Header().Set("Content-Type", out.ContentType())
	if err := search.SendFromReader(out, rc); err != nil {
		c.requestLogger(r).Warn("Error writing response", zap.Error(err))
	}
}

func extractSpace(c *Core, w http.ResponseWriter, r *http.Request) space.Space {
	v := mux.Vars(r)
	id, ok := v["space"]
	if !ok {
		respondError(c, w, r, zqe.E(zqe.Invalid, "no space id in path"))
		return nil
	}
	s, err := c.spaces.Get(api.SpaceID(id))
	if err != nil {
		respondError(c, w, r, err)
		return nil
	}
	return s
}
