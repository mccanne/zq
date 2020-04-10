package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/brimsec/zq/proc"
	"github.com/brimsec/zq/zbuf"
	"github.com/brimsec/zq/zqd/api"
)

type Driver interface {
	Warn(msg string) error
	Write(channelID int, batch zbuf.Batch) error
	ChannelEnd(channelID int, batch api.ScannerStats) error
	Stats(api.ScannerStats) error
}

func Run(out *proc.MuxOutput, d Driver, statsInterval time.Duration) error {
	//stats are zero at this point.
	var stats api.ScannerStats
	ticker := time.NewTicker(statsInterval)
	defer ticker.Stop()
	for !out.Complete() {
		chunk := out.Pull(ticker.C)
		if chunk.Err != nil {
			if chunk.Err == proc.ErrTimeout {
				/* not yet
				err := d.sendStats(out.Stats())
				if err != nil {
					return d.abort(0, err)
				}
				*/
				continue
			}
			if chunk.Err == context.Canceled {
				out.Drain()
			}
			return chunk.Err
		}
		if chunk.Warning != "" {
			if err := d.Warn(chunk.Warning); err != nil {
				return err
			}
		}
		if chunk.Batch == nil {
			// One of the flowgraph tails is done.  We send stats and
			// a done message for each channel that finishes
			if err := d.ChannelEnd(chunk.ID, stats); err != nil {
				return err
			}
		} else {
			if err := d.Write(chunk.ID, chunk.Batch); err != nil {
				return err
			}
		}
	}
	return nil
}

// CLI implements Driver
type CLI struct {
	writers  []zbuf.Writer
	warnings io.Writer
}

func NewCLI(w ...zbuf.Writer) *CLI {
	return &CLI{
		writers: w,
	}
}

func (d *CLI) SetWarningsWriter(w io.Writer) {
	d.warnings = w
}

func (d *CLI) Write(cid int, arr zbuf.Batch) error {
	if len(d.writers) == 1 {
		cid = 0
	}
	for _, r := range arr.Records() {
		if err := d.writers[cid].Write(r); err != nil {
			return err
		}
	}
	return nil
}

func (d *CLI) Warn(msg string) error {
	if d.warnings != nil {
		_, err := fmt.Fprintln(d.warnings, msg)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *CLI) ChannelEnd(int, api.ScannerStats) error { return nil }
func (d *CLI) Stats(api.ScannerStats) error           { return nil }
