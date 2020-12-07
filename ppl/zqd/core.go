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

	Auth     AuthConfig
	Suricata pcapanalyzer.Launcher
	Zeek     pcapanalyzer.Launcher
}

type middleware interface {
	Middleware(next http.Handler) http.Handler
}

type Core struct {
	auth      middleware
	logger    *zap.Logger
	mgr       *apiserver.Manager
	registry  *prometheus.Registry
	root      iosrc.URI
	router    *mux.Router
	taskCount int64

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

	auth, err := newAuthenticator(ctx, conf.Logger, registry, conf.Auth)
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
		auth:     auth,
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
	case "worker":
		c.addWorkerRoutes()
	default:
		return nil, fmt.Errorf("unknown personality %s", conf.Personality)
	}

	return c, nil
}

func (c *Core) addAPIServerRoutes() {
	c.authhandle("/ast", handleASTPost).Methods("POST")
	c.authhandle("/auth/identity", handleIdentityGet).Methods("GET")
	c.authhandle("/search", handleSearch).Methods("POST")
	c.authhandle("/space", handleSpaceList).Methods("GET")
	c.authhandle("/space", handleSpacePost).Methods("POST")
	c.authhandle("/space/{space}", handleSpaceDelete).Methods("DELETE")
	c.authhandle("/space/{space}", handleSpaceGet).Methods("GET")
	c.authhandle("/space/{space}", handleSpacePut).Methods("PUT")
	c.authhandle("/space/{space}/archivestat", handleArchiveStat).Methods("GET")
	c.authhandle("/space/{space}/index", handleIndexPost).Methods("POST")
	c.authhandle("/space/{space}/indexsearch", handleIndexSearch).Methods("POST")
	c.authhandle("/space/{space}/log", handleLogStream).Methods("POST")
	c.authhandle("/space/{space}/log/paths", handleLogPost).Methods("POST")
	c.authhandle("/space/{space}/pcap", handlePcapPost).Methods("POST")
	c.authhandle("/space/{space}/pcap", handlePcapSearch).Methods("GET")
	c.authhandle("/space/{space}/subspace", handleSubspacePost).Methods("POST")
}

func (c *Core) addWorkerRoutes() {
	c.handle("/worker", handleWorker).Methods("POST")
}

func coreHandler(c *Core, f func(*Core, http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f(c, w, r)
	})
}

func (c *Core) handle(path string, f func(*Core, http.ResponseWriter, *http.Request)) *mux.Route {
	return c.router.Handle(path, coreHandler(c, f))
}

func (c *Core) authhandle(path string, f func(*Core, http.ResponseWriter, *http.Request)) *mux.Route {
	return c.router.Handle(path, c.auth.Middleware(coreHandler(c, f)))
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
