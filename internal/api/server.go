package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/p2p-claude-plans/p2p-claude-plans/internal/config"
	"github.com/p2p-claude-plans/p2p-claude-plans/internal/node"
	"github.com/p2p-claude-plans/p2p-claude-plans/internal/planstore"
)

type Server struct {
	store    *planstore.Store
	node     *node.Node
	peerName string
	timeout  time.Duration
	httpSrv  *http.Server
}

func NewServer(store *planstore.Store, cfg *config.Config) *Server {
	s := &Server{
		store:    store,
		peerName: cfg.PeerName,
		timeout:  time.Duration(cfg.RequestTimeout) * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /plans", s.handleListPlans)
	mux.HandleFunc("GET /plans/{peerID}/{planID}", s.handleGetPlan)
	mux.HandleFunc("GET /peers", s.handleListPeers)
	mux.HandleFunc("GET /health", s.handleHealth)

	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", cfg.HTTPPort),
		Handler: mux,
	}
	return s
}

func (s *Server) SetNode(n *node.Node) {
	s.node = n
}

func (s *Server) ListenAndServe() error {
	return s.httpSrv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
