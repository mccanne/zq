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
	unreservereq := api.UnreserveRequest{
		Addrs: []string{w.selfaddr},
	}
	resp1, err := w.conn.Unreserve(ctx, unreservereq)
	if err != nil {
		return fmt.Errorf("error on unreserve with recruiter at %s : %v", w.recruiteraddr, err)
	}
	if resp1.Reserved != false {
		return fmt.Errorf("recruiter did not acknowlege unreserve")
	}

	registerreq := api.RegisterRequest{
		Worker: api.Worker{
			Addr:     w.selfaddr,
			NodeName: w.nodename,
		},
	}
	resp2, err := w.conn.Register(ctx, registerreq)
	if err != nil {
		return fmt.Errorf("error on register with recruiter at %s : %v", w.recruiteraddr, err)
	}
	if resp2.Registered != true {
		return fmt.Errorf("recruiter did not acknowlege register")
	}
	logger.Info(
		"Registered",
		zap.String("selfaddr", w.selfaddr),
		zap.String("recruiteraddr", w.recruiteraddr),
		zap.String("nodename", w.nodename),
	)
	return nil
}
