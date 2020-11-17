package recruit

import (
	"flag"
	"io"
	"net"
	"net/http"

	"github.com/brimsec/zq/cli"
	"github.com/brimsec/zq/pkg/fs"
	"github.com/brimsec/zq/pkg/httpd"
	"github.com/brimsec/zq/ppl/cmd/zqd/logger"
	"github.com/brimsec/zq/ppl/cmd/zqd/root"
	"github.com/brimsec/zq/ppl/zqd"
	"github.com/mccanne/charm"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Recruit = &charm.Spec{
	Name:  "recruit",
	Usage: "recruit [options]",
	Short: "listen as a daemon and repond to requests from other zqd listeners",
	Long: `
	zqd recruit manages a pool of zqd workers and provides a mechanism for zqd root processes to recruit zqd worker processes for query execution.
`,
	//HiddenFlags: "brimfd,workers,svc",
	New: New,
}

func init() {
	root.Zqd.Add(Recruit)
}

type Command struct {
	*root.Command
	listenAddr string
	conf       zqd.Config
	pprof      bool
	prom       bool
	configfile string
	loggerConf *logger.Config
	logLevel   zapcore.Level
	logger     *zap.Logger
	devMode    bool
	portFile   string
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{Command: parent.(*root.Command)}
	c.conf.Version = cli.Version
	f.StringVar(&c.listenAddr, "l", ":9867", "[addr]:port to listen on")
	f.BoolVar(&c.prom, "prometheus", false, "add prometheus metrics routes to api")
	f.StringVar(&c.configfile, "config", "", "path to a zqd config file")
	f.Var(&c.logLevel, "loglevel", "level for log output (defaults to info)")
	f.BoolVar(&c.devMode, "dev", false, "runs zqd in development mode")
	f.StringVar(&c.portFile, "portfile", "", "write port of http listener to file")
	return c, nil
}

func (c *Command) Run(args []string) error {
	defer c.Cleanup()
	if err := c.Init(); err != nil {
		return err
	}
	if err := c.init(); err != nil {
		return err
	}
	c.logger.Info("Starting")

	h := zqd.NewHandler(core, c.logger)
	srv := httpd.New(c.listenAddr, h)
	srv.SetLogger(c.logger.Named("httpd"))
	if err := srv.Start(ctx); err != nil {
		return err
	}
	return srv.Wait()
}

func (c *Command) init() error {
	return c.initLogger()
}

// XXX Eventually this function should take prometheus.Registry as an argument.
// For now since we only care about retrieving go stats, create registry
// here.
func prometheusHandlers(h http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", h)
	promreg := prometheus.NewRegistry()
	promreg.MustRegister(prometheus.NewGoCollector())
	promhandler := promhttp.HandlerFor(promreg, promhttp.HandlerOpts{})
	mux.Handle("/metrics", promhandler)
	return mux
}

// defaultLogger ignores output from the access logger.
func (c *Command) defaultLogger() *logger.Config {
	return &logger.Config{
		Type: logger.TypeWaterfall,
		Children: []logger.Config{
			{
				Name:  "http.access",
				Path:  "/dev/null",
				Level: c.logLevel,
			},
			{
				Path:  "stderr",
				Level: c.logLevel,
			},
		},
	}
}

func (c *Command) initLogger() error {
	if c.loggerConf == nil {
		c.loggerConf = c.defaultLogger()
	}
	core, err := logger.NewCore(*c.loggerConf)
	if err != nil {
		return err
	}
	// If the development mode is on, calls to logger.DPanic will cause a panic
	// whereas in production would result in an error.
	var opts []zap.Option
	if c.devMode {
		opts = append(opts, zap.Development())
	}
	c.logger = zap.New(core, opts...)
	c.conf.Logger = c.logger
	return nil
}

func (c *Command) writePortFile(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	return fs.ReplaceFile(c.portFile, 0644, func(w io.Writer) error {
		_, err := w.Write([]byte(port))
		return err
	})
}
