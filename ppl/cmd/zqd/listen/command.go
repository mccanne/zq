package listen

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"runtime"

	"github.com/brimsec/zq/cli"
	"github.com/brimsec/zq/pkg/fs"
	"github.com/brimsec/zq/pkg/httpd"
	"github.com/brimsec/zq/pkg/rlimit"
	"github.com/brimsec/zq/ppl/cmd/zqd/logger"
	"github.com/brimsec/zq/ppl/cmd/zqd/root"
	"github.com/brimsec/zq/ppl/zqd"
	"github.com/brimsec/zq/ppl/zqd/pcapanalyzer"
	"github.com/brimsec/zq/proc/sort"
	"github.com/mccanne/charm"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

var Listen = &charm.Spec{
	Name:  "listen",
	Usage: "listen [options]",
	Short: "listen as a daemon and repond to zqd service requests",
	Long: `
The listen command launches a process to listen on the provided interface and
`,
	HiddenFlags: "brimfd,workers,recruiter,podip,nodename",
	New:         New,
}

func init() {
	root.Zqd.Add(Listen)
}

type Command struct {
	*root.Command
	conf            zqd.Config
	logger          *zap.Logger
	loggerConf      *logger.Config
	suricataUpdater pcapanalyzer.Launcher

	// Flags

	// brimfd is a file descriptor passed through by brim desktop. If set zqd
	// will exit if the fd is closed.
	brimfd              int
	configfile          string
	devMode             bool
	listenAddr          string
	logLevel            zapcore.Level
	portFile            string
	pprof               bool
	suricataRunnerPath  string
	suricataUpdaterPath string
	zeekRunnerPath      string
}

func New(parent charm.Command, f *flag.FlagSet) (charm.Command, error) {
	c := &Command{Command: parent.(*root.Command)}
	c.conf.Version = cli.Version
	f.IntVar(&c.brimfd, "brimfd", -1, "pipe read fd passed by brim to signal brim closure")
	f.StringVar(&c.configfile, "config", "", "path to zqd config file")
	f.StringVar(&c.conf.Root, "data", ".", "data location")
	f.BoolVar(&c.devMode, "dev", false, "run in development mode")
	f.StringVar(&c.listenAddr, "l", ":9867", "[addr]:port to listen on")
	f.Var(&c.logLevel, "loglevel", "logging level")
	f.StringVar(&c.conf.Personality, "personality", "all", "server personality (all, apiserver, recruiter, or worker)")
	f.StringVar(&c.portFile, "portfile", "", "write listen port to file")
	f.BoolVar(&c.pprof, "pprof", false, "add pprof routes to API")
	f.StringVar(&c.conf.Recruiter, "recruiter", "", "Recruiter address for worker registration")
	f.StringVar(&c.conf.SpecPodIP, "podip", "", "IP provided by Kubernetes SPEC_POD_IP")
	f.StringVar(&c.conf.SpecNodeName, "nodename", "", "Node name provided by Kubernetes SPEC_NODE_NAME")
	f.StringVar(&c.suricataRunnerPath, "suricatarunner", "", "command to generate Suricata eve.json from pcap data")
	f.StringVar(&c.suricataUpdaterPath, "suricataupdater", "", "command to update Suricata rules (run once at startup)")
	f.StringVar(&c.conf.Workers, "workers", "", "workers as comma-separated [addr]:port list")
	f.StringVar(&c.zeekRunnerPath, "zeekrunner", "", "command to generate Zeek logs from pcap data")
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
	openFilesLimit, err := rlimit.RaiseOpenFilesLimit()
	if err != nil {
		c.logger.Warn("Raising open files limit failed", zap.Error(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if c.brimfd != -1 {
		if ctx, err = c.watchBrimFd(ctx); err != nil {
			return err
		}
	}
	core, err := zqd.NewCore(ctx, c.conf)
	if err != nil {
		return err
	}
	defer core.Shutdown()
	c.logger.Info("Starting",
		zap.String("datadir", c.conf.Root),
		zap.Uint64("open_files_limit", openFilesLimit),
		zap.String("personality", c.conf.Personality),
		zap.Bool("pprof_routes", c.pprof),
		zap.Bool("suricata_supported", core.HasSuricata()),
		zap.Bool("zeek_supported", core.HasZeek()),
	)
	h := core.HTTPHandler()
	if c.pprof {
		h = pprofHandlers(h)
	}
	if c.suricataUpdater != nil {
		c.launchSuricataUpdate(ctx)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	go func() {
		sig := <-ch
		c.logger.Info("Signal received", zap.Stringer("signal", sig))
		cancel()
	}()
	srv := httpd.New(c.listenAddr, h)
	srv.SetLogger(c.logger.Named("httpd"))
	if err := srv.Start(ctx); err != nil {
		return err
	}
	// Workers should registerWithRecruiter as late as possible,
	// just before writing Port file for tests.
	if c.conf.Personality == "worker" {
		if _, _, err := net.SplitHostPort(c.conf.Recruiter); err != nil {
			return errors.New("flag -recruiter=host:port must be provided for -personality=worker")
		}
		if c.conf.SpecNodeName == "" {
			return errors.New("flag -nodename must be provided for -personality=worker")
		}
		if err := core.WorkerRegistration(ctx, srv.Addr(), c.conf); err != nil {
			return err
		}
	}
	if c.portFile != "" {
		if err := c.writePortFile(srv.Addr()); err != nil {
			return err
		}
	}
	return srv.Wait()
}

func (c *Command) init() error {
	if err := c.loadConfigFile(); err != nil {
		return err
	}
	if err := c.initLogger(); err != nil {
		return err
	}
	var err error
	c.conf.Suricata, err = getLauncher(c.suricataRunnerPath, "suricatarunner", false)
	if err != nil {
		return err
	}
	c.suricataUpdater, err = getLauncher(c.suricataUpdaterPath, "suricataupdater", true)
	if err != nil {
		return err
	}
	c.conf.Zeek, err = getLauncher(c.zeekRunnerPath, "zeekrunner", false)
	return err
}

func getLauncher(path, defaultFile string, stdout bool) (pcapanalyzer.Launcher, error) {
	if path == "" {
		var err error
		if path, err = exec.LookPath(defaultFile); err != nil {
			return nil, nil
		}
	}
	return pcapanalyzer.LauncherFromPath(path, stdout)
}

func (c *Command) watchBrimFd(ctx context.Context) (context.Context, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("flag -brimfd not applicable to windows")
	}
	f := os.NewFile(uintptr(c.brimfd), "brimfd")
	c.logger.Info("Listening to brim process pipe", zap.String("fd", f.Name()))
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		io.Copy(ioutil.Discard, f)
		c.logger.Info("Brim fd closed, shutting down")
		cancel()
	}()
	return ctx, nil
}

func pprofHandlers(h http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

// Example configfile
// logger:
//   type: waterfall
//   children:
//   - path: ./data/access.log
//     name: "http.access"
//     level: info
//     mode: truncate
// sort_mem_max_bytes: 268432640

func (c *Command) loadConfigFile() error {
	if c.configfile == "" {
		return nil
	}
	conf := &struct {
		Logger          logger.Config `yaml:"logger"`
		SortMemMaxBytes *int          `yaml:"sort_mem_max_bytes,omitempty"`
	}{}
	b, err := ioutil.ReadFile(c.configfile)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(b, conf)
	c.loggerConf = &conf.Logger
	if v := conf.SortMemMaxBytes; v != nil {
		if *v <= 0 {
			return fmt.Errorf("%s: sortMemMaxBytes value must be greater than zero", c.configfile)
		}
		sort.MemMaxBytes = *v
	}

	return err
}

func (c *Command) launchSuricataUpdate(ctx context.Context) {
	c.logger.Info("Launching suricata updater")
	go func() {
		sproc, err := c.suricataUpdater(ctx, nil, "")
		if err != nil {
			c.logger.Error("Launching suricata updater", zap.Error(err))
			return
		}
		err = sproc.Wait()
		c.logger.Info("Suricata updater completed")
		if err != nil {
			c.logger.Error("Running suricata updater", zap.Error(err))
			return
		}
		stdout := sproc.Stdout()
		c.logger.Info("Suricata updater stdout", zap.String("stdout", stdout))
	}()
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
