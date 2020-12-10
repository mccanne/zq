package zqd

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/ppl/zqd/recruiter"
	"github.com/brimsec/zq/zqe"
	"go.uber.org/zap"
)

func handleRecruit(c *Core, w http.ResponseWriter, r *http.Request) {
	var req api.RecruitRequest
	if !request(c, w, r, &req) {
		return
	}
	ws, err := c.workerPool.Recruit(req.NumberRequested)
	if err != nil {
		respondError(c, w, r, zqe.ErrInvalid(err))
		return
	}
	workers := make([]api.Worker, len(ws))
	for i, e := range ws {
		e.Recruited <- recruiter.RecruitmentDetail{Label: req.Label, NumberRequested: req.NumberRequested}
		workers[i] = api.Worker{Addr: e.Addr, NodeName: e.NodeName}
	}
	respond(c, w, r, http.StatusOK, api.RecruitResponse{
		Workers: workers,
	})
}

func handleRegister(c *Core, w http.ResponseWriter, r *http.Request) {
	var req api.RegisterRequest
	if !request(c, w, r, &req) {
		return
	}
	if req.RequestedTimeout <= 0 {
		respondError(c, w, r, zqe.E(zqe.Invalid, "required parameter RequestedTimeout"))
		return
	}

	recruited := make(chan recruiter.RecruitmentDetail)

	if err := c.workerPool.Register(req.Addr, req.NodeName, recruited); err != nil {
		respondError(c, w, r, zqe.ErrInvalid(err))
		return
	}

	var isCanceled bool
	var directive api.WorkerDirective

	ctx := r.Context()
	timer := time.NewTimer(time.Duration(req.RequestedTimeout) * time.Millisecond)
	defer timer.Stop()
	select {
	case rd := <-recruited:
		c.requestLogger(r).Info("Worker recruited",
			zap.String("addr", req.Addr),
			zap.String("label", rd.Label),
			zap.Int("count", rd.NumberRequested),
		)
		directive = api.Reserved
	case <-timer.C:
		c.requestLogger(r).Info("Worker should reregister", zap.String("addr", req.Addr))
		directive = api.Reregister
	case <-ctx.Done():
		c.requestLogger(r).Info("handleRegister context cancel")
		isCanceled = true
	}

	// Deregister in any event
	c.workerPool.Deregister(req.Addr)

	// Future: add logic to scale down by responding with "shutdown"
	// We would check to see if the worker is on an unscedulable node,
	// and direct it to shutdown if needed.
	// Note that I expect to need rd.NumberRequested for a scaling heuristic. -MTW

	if !isCanceled {
		respond(c, w, r, http.StatusOK, api.RegisterResponse{
			Directive: directive,
		})
	}
}

func handleRecruiterStats(c *Core, w http.ResponseWriter, r *http.Request) {
	respond(c, w, r, http.StatusOK, api.RecruiterStatsResponse{
		LenFreePool:     c.workerPool.LenFreePool(),
		LenReservedPool: c.workerPool.LenReservedPool(),
		LenNodePool:     c.workerPool.LenNodePool(),
	})
}

// handleListFree pretty prints the output because it is for manual trouble-shooting.
func handleListFree(c *Core, w http.ResponseWriter, r *http.Request) {
	ws := c.workerPool.ListFreePool()
	workers := make([]api.Worker, len(ws))
	for i, e := range ws {
		workers[i] = api.Worker{Addr: e.Addr, NodeName: e.NodeName}
	}
	body := api.RecruitResponse{
		Workers: workers,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		c.requestLogger(r).Warn("Error writing response", zap.Error(err))
	}
}
