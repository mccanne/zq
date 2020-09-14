package zeek

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/brimsec/zq/zqd/process"
)

// LauncherFromPath returns a Launcher instance that will execute a pcap
// to zeek log transformation, using the provided path to the command.
// zeekpath should point to an executable or script that:
// - expects to receive a pcap file on stdin
// - writes the resulting zeek logs into its working directory
func LauncherFromPath(zeekpath string) (process.Launcher, error) {
	var cmdline []string

	if runtime.GOOS == "windows" {
		// On windows, use the hidden zqd subcommand winexec that ensures any
		// spawned process is terminated.
		zqdexec, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("cant get executable path for zqd")
		}
		cmdline = []string{zqdexec, "winexec", zeekpath}
	} else {
		cmdline = []string{zeekpath}
	}

	return func(ctx context.Context, r io.Reader, dir string) (process.ProcessWaiter, error) {
		cmd := exec.CommandContext(ctx, cmdline[0], cmdline[1:]...)
		cmd.Dir = dir
		cmd.Stdin = r
		p := process.NewProcess(cmd, "zeek")
		return p, p.Start()
	}, nil
}
