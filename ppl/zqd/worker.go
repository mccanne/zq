package zqd

import (
	"net/http"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/ppl/archive"
	"github.com/brimsec/zq/ppl/zqd/search"
	"github.com/brimsec/zq/ppl/zqd/storage/archivestore"
	"github.com/brimsec/zq/zqe"
	"go.uber.org/zap"
)

func handleWorkerRootSearch(c *Core, w http.ResponseWriter, r *http.Request) {
	var req api.WorkerRootRequest
	if !request(c, w, r, &req) {
		return
	}

	if req.MaxWorkers < 1 || req.MaxWorkers > 100 {
		// Note: the upper limit of distributed workers for any given query
		// will be determined based on future testing.
		// Hard coded for now to 100 for intial testing and research.
		// It will be based the size of the worker cluster, and will be
		// included in the environment.
		err := zqe.ErrInvalid("number of workers requested must be between 1 and 100")
		respondError(c, w, r, err)
		return
	}

	srch, err := search.NewSearchOp(req.SearchRequest)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	out, err := getSearchOutput(w, r)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	store, err := c.mgr.GetStorage(r.Context(), req.SearchRequest.Space)
	if err != nil {
		respondError(c, w, r, err)
		return
	}

	w.Header().Set("Content-Type", out.ContentType())
	if err := srch.RunDistributed(r.Context(), store, out, req.MaxWorkers, c.recruiter, c.workers); err != nil {
		c.requestLogger(r).Warn("Error writing response", zap.Error(err))
	}
}

func handleWorkerChunkSearch(c *Core, w http.ResponseWriter, httpReq *http.Request) {
	var req api.WorkerChunkRequest
	if !request(c, w, httpReq, &req) {
		return
	}

	ctx := httpReq.Context()

	ark, err := archive.OpenArchiveWithContext(ctx, req.DataPath, &archive.OpenOptions{})
	if err != nil {
		respondError(c, w, httpReq, err)
		return
	}

	work, err := search.NewWorkerOp(ctx, req, archivestore.NewStorage(ark))
	if err != nil {
		respondError(c, w, httpReq, err)
		return
	}

	out, err := getSearchOutput(w, httpReq)
	if err != nil {
		respondError(c, w, httpReq, err)
		return
	}

	w.Header().Set("Content-Type", out.ContentType())

	if err := work.Run(ctx, out); err != nil {
		c.requestLogger(httpReq).Warn("Error writing response", zap.Error(err))
	}
}

func handleWorkerRelease(c *Core, w http.ResponseWriter, httpReq *http.Request) {
	w.WriteHeader(http.StatusNoContent)
	if err := c.workerReg.RegisterWithRecruiter(httpReq.Context(), c.logger); err != nil {
		// No point in responding with the error back to zqd root process,
		// since this is happening on the cleanup after the search is finished.
		c.logger.Warn("WorkerReleaseError", zap.Error(err))
	}
}
