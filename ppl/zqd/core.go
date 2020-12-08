package zqd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/ppl/zqd/apiserver"
	"github.com/brimsec/zq/ppl/zqd/pcapanalyzer"
	"github.com/brimsec/zq/ppl/zqd/recruiter"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const indexPage = `
<!DOCTYPE html>
<html>
  <title>ZQD daemon</title>
  <body style="padding:10px">
    <h2>ZQD</h2>
    <p>A <a href="https://github.com/brimsec/zq/tree/master/cmd/zqd">zqd</a> daemon is listening on this host/port.</p>
    <p>If you're a <a href="https://www.brimsecurity.com/">Brim</a> user, connect to this host/port from the <a href="https://github.com/brimsec/brim">Brim application</a> in the graphical desktop interface in your operating system (not a web browser).</p>
    <p>If your goal is to perform command line operations against this zqd, use the <a href="https://github.com/brimsec/zq/tree/master/cmd/zapi">zapi</a> client.</p>
  </body>
</html>`

type Config struct {
	Logger      *zap.Logger
	Personality string
	Root        string
	Version     string

	Suricata pcapanalyzer.Launcher
	Zeek     pcapanalyzer.Launcher
}

type Core struct {
	logger     *zap.Logger
	mgr        *apiserver.Manager
	registry   *prometheus.Registry
	root       iosrc.URI
	router     *mux.Router
	taskCount  int64
	workerPool *recruiter.WorkerPool // state for personality=recruiter
	workerReg  *recruiter.WorkerReg  // state for personality=worker

	suricata pcapanalyzer.Launcher
	zeek     pcapanalyzer.Launcher
}

func NewCore(ctx context.Context, conf Config) (*Core, error) {
	if conf.Logger == nil {
		conf.Logger = zap.NewNop()
	}
	if conf.Version == "" {
		conf.Version = "unknown"
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewGoCollector())

	root, err := iosrc.ParseURI(conf.Root)
	if err != nil {
		return nil, err
	}

	mgr, err := apiserver.NewManager(ctx, conf.Logger, registry, root)
	if err != nil {
		return nil, err
	}

	router := mux.NewRouter()
	router.Use(requestIDMiddleware())
	router.Use(accessLogMiddleware(conf.Logger))
	router.Use(panicCatchMiddleware(conf.Logger))
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, indexPage)
	})
	router.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	router.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ok")
	})
	router.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&api.VersionResponse{Version: conf.Version})
	})

	c := &Core{
		logger:   conf.Logger,
		mgr:      mgr,
		registry: registry,
		root:     root,
		router:   router,
		suricata: conf.Suricata,
		zeek:     conf.Zeek,
	}

	switch conf.Personality {
	case "", "all":
		c.addAPIServerRoutes()
		c.addWorkerRoutes()
	case "apiserver":
		c.addAPIServerRoutes()
	case "recruiter":
		c.workerPool = recruiter.NewWorkerPool()
		c.addRecruiterRoutes()
	case "worker":
		c.addWorkerRoutes()
	default:
		return nil, fmt.Errorf("unknown personality %s", conf.Personality)
	}

	return c, nil
}

func (c *Core) addAPIServerRoutes() {
	c.handle("/ast", handleASTPost).Methods("POST")
	c.handle("/search", handleSearch).Methods("POST")
	c.handle("/space", handleSpaceList).Methods("GET")
	c.handle("/space", handleSpacePost).Methods("POST")
	c.handle("/space/{space}", handleSpaceDelete).Methods("DELETE")
	c.handle("/space/{space}", handleSpaceGet).Methods("GET")
	c.handle("/space/{space}", handleSpacePut).Methods("PUT")
	c.handle("/space/{space}/archivestat", handleArchiveStat).Methods("GET")
	c.handle("/space/{space}/index", handleIndexPost).Methods("POST")
	c.handle("/space/{space}/indexsearch", handleIndexSearch).Methods("POST")
	c.handle("/space/{space}/log", handleLogStream).Methods("POST")
	c.handle("/space/{space}/log/paths", handleLogPost).Methods("POST")
	c.handle("/space/{space}/pcap", handlePcapPost).Methods("POST")
	c.handle("/space/{space}/pcap", handlePcapSearch).Methods("GET")
	c.handle("/space/{space}/subspace", handleSubspacePost).Methods("POST")
}

func (c *Core) addRecruiterRoutes() {
	c.handle("/recruiter/deregister", handleDeregister).Methods("POST")
	c.handle("/recruiter/listfree", handleListFree).Methods("GET")
	c.handle("/recruiter/longpollregister", handleLongPollRegister).Methods("POST")
	c.handle("/recruiter/recruit", handleRecruit).Methods("POST")
	c.handle("/recruiter/register", handleRegister).Methods("POST")
	c.handle("/recruiter/stats", handleRecruiterStats).Methods("GET")
	c.handle("/recruiter/unreserve", handleUnreserve).Methods("POST")
}

func (c *Core) addWorkerRoutes() {
	c.handle("/worker/release", handleWorkerRelease).Methods("GET")
	c.handle("/worker/rootsearch", handleWorkerRootSearch).Methods("POST")
	c.handle("/worker/chunksearch", handleWorkerChunkSearch).Methods("POST")
}

func (c *Core) handle(path string, f func(*Core, http.ResponseWriter, *http.Request)) *mux.Route {
	return c.router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		f(c, w, r)
	})
}

func (c *Core) HTTPHandler() http.Handler {
	return c.router
}

func (c *Core) HasSuricata() bool {
	return c.suricata != nil
}

func (c *Core) HasZeek() bool {
	return c.zeek != nil
}

func (c *Core) Registry() *prometheus.Registry {
	return c.registry
}

func (c *Core) Root() iosrc.URI {
	return c.root
}

func (c *Core) Shutdown() {
	c.mgr.Shutdown()
}

func (c *Core) nextTaskID() int64 {
	return atomic.AddInt64(&c.taskCount, 1)
}

func (c *Core) requestLogger(r *http.Request) *zap.Logger {
	return c.logger.With(zap.String("request_id", getRequestID(r.Context())))
}

func (c *Core) WorkerRegistration(ctx context.Context, srvAddr string) error {
	var err error
	c.workerReg, err = recruiter.NewWorkerReg(ctx, srvAddr)
	if err != nil {
		return err
	}
	return c.workerReg.RegisterWithRecruiter(ctx, c.logger)
}
