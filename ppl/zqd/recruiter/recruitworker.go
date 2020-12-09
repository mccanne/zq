package recruiter

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/api/client"
	"github.com/brimsec/zq/proc"
	"go.uber.org/zap"
)

func RecruitWorkers(ctx *proc.Context, workerCount int) ([]string, error) {
	if workerstr := os.Getenv("ZQD_TEST_WORKERS"); workerstr != "" {
		// Special case: ZQD_TEST_WORKERS is used for ZTests
		workers := strings.Split(workerstr, ",")
		if workerCount > len(workers) {
			return nil, fmt.Errorf("requested parallelism %d is greater than the number of workers %d",
				workerCount, len(workers))
		}
		for _, w := range workers {
			if _, _, err := net.SplitHostPort(w); err != nil {
				return nil, err
			}
		}
		return workers, nil
	}

	var raddr string
	if raddr = os.Getenv("ZQD_RECRUITER_ADDR"); raddr == "" {
		return nil, fmt.Errorf("distributed exec failure: ZQD_RECRUITER_ADDR not present")
	}
	if _, _, err := net.SplitHostPort(raddr); err != nil {
		return nil, fmt.Errorf("distributed exec failure: ZQD_RECRUITER_ADDR for root process does not have host:port")
	}
	conn := client.NewConnectionTo("http://" + raddr)
	recreq := api.RecruitRequest{NumberRequested: workerCount}
	resp, err := conn.Recruit(ctx, recreq)
	if err != nil {
		return nil, fmt.Errorf("distributed exec failure: error on recruit for recruiter at %s : %v", raddr, err)
	}
	if workerCount > len(resp.Workers) {
		// TODO: we should fail back to running the query with fewer workers if possible.
		// Determining when that is possible is non-trivial. One issue is that the
		// parallel procs have already been compiled into the flowgraph by the time we get here.
		// An alternative is to wait and try to recruit more workers,
		// which would reserve the idle zqd root process while waiting. -MTW
		return nil, fmt.Errorf("distributed exec failure: requested workers %d greater than available workers %d",
			workerCount, len(resp.Workers))
	}

	var workers []string
	for _, w := range resp.Workers {
		workers = append(workers, w.Addr)
	}
	return workers, nil
}

func ReleaseWorker(ctx context.Context, conn *client.Connection, logger *zap.Logger) error {
	logger.Info("ReleaseWorker", zap.String("addr", conn.ClientHostURL()))
	return conn.WorkerRelease(ctx)
}
