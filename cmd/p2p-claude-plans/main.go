package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/p2p-claude-plans/p2p-claude-plans/internal/api"
	"github.com/p2p-claude-plans/p2p-claude-plans/internal/config"
	"github.com/p2p-claude-plans/p2p-claude-plans/internal/node"
	"github.com/p2p-claude-plans/p2p-claude-plans/internal/planstore"
	"github.com/p2p-claude-plans/p2p-claude-plans/internal/protocol"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "keygen" {
		if err := node.GenerateSwarmKey(os.Stdout); err != nil {
			log.Fatalf("keygen: %v", err)
		}
		return
	}

	configPath := flag.String("config", "", "path to config file")
	bootstrapMode := flag.Bool("bootstrap-mode", false, "run as DHT bootstrap server")
	listenAddr := flag.String("listen", "", "libp2p listen address (overrides config)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Config: %v", err)
	}
	if *bootstrapMode {
		cfg.BootstrapMode = true
	}
	if *listenAddr != "" {
		cfg.ListenAddrs = []string{*listenAddr}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store := planstore.New(cfg.PlansDir)
	if err := store.Watch(ctx); err != nil {
		log.Fatalf("Plan store: %v", err)
	}

	n, err := node.NewNode(ctx, cfg)
	if err != nil {
		log.Fatalf("Node: %v", err)
	}
	defer n.Close()

	protocol.RegisterHandler(n.Host, store, cfg.PeerName)

	log.Printf("Peer ID: %s", n.PeerID())
	for _, addr := range n.Addrs() {
		fmt.Fprintf(os.Stderr, "  %s/p2p/%s\n", addr, n.PeerID())
	}

	apiServer := api.NewServer(store, cfg)
	apiServer.SetNode(n)
	go func() {
		log.Printf("HTTP API on 127.0.0.1:%d", cfg.HTTPPort)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	apiServer.Shutdown(shutdownCtx)
}
