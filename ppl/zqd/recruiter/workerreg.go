package recruiter

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/api/client"
	"go.uber.org/zap"
)

type WorkerReg struct {
	conn          *client.Connection
	recruiteraddr string
	selfaddr      string
	nodename      string
}

func NewWorkerReg(ctx context.Context, srvAddr string) (*WorkerReg, error) {
	w := &WorkerReg{}
	w.recruiteraddr = os.Getenv("ZQD_RECRUITER_ADDR")
	if _, _, err := net.SplitHostPort(w.recruiteraddr); err != nil {
		return nil, fmt.Errorf("worker ZQD_RECRUITER_ADDR does not have host:port %v", err)
	}
	w.conn = client.NewConnectionTo("http://" + w.recruiteraddr)
	// For server host and port, the environment variables will override the discovered address.
	// This allows the deployment to specify a dns address provided by the K8s API rather than an IP.
	host, port, _ := net.SplitHostPort(srvAddr)
	if h := os.Getenv("ZQD_POD_IP"); h != "" {
		host = h
	}
	if p := os.Getenv("ZQD_PORT"); p != "" {
		port = p
	}
	w.selfaddr = net.JoinHostPort(host, port)
	w.nodename = os.Getenv("ZQD_NODE_NAME")
	if w.nodename == "" {
		return nil, fmt.Errorf("env var ZQD_NODE_NAME required to register with recruiter")
	}
	return w, nil
}

func (w *WorkerReg) RegisterWithRecruiter(ctx context.Context, logger *zap.Logger) error {
	// This should be a loop that tries to reregister, called as a goroutine.
	// Loop should be suspended when a /worker/search is in progress, and
	// resume afterwards.
	// So, break out of loop when reserved, then register is called again on /worker/release
	// Failure case is when /worker/release is not called. Maybe we need some locks and timers
	// to take care of that.

	registerreq := api.RegisterRequest{
		Worker: api.Worker{
			Addr:     w.selfaddr,
			NodeName: w.nodename,
		},
	}
	// this will be a long poll:
	resp, err := w.conn.Register(ctx, registerreq)
	if err != nil {
		return fmt.Errorf("error on register with recruiter at %s : %v", w.recruiteraddr, err)
	}

	// various logic based on directive here

	logger.Info(
		"Registered response",
		zap.Int("directive", int(resp.Directive)),
		zap.String("selfaddr", w.selfaddr),
		zap.String("recruiteraddr", w.recruiteraddr),
		zap.String("nodename", w.nodename),
	)
	return nil
}
