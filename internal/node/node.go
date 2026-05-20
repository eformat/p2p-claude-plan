package node

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/p2p-claude-plans/p2p-claude-plans/internal/config"
)

type Node struct {
	Host   host.Host
	DHT    *dht.IpfsDHT
	Config *config.Config
}

func NewNode(ctx context.Context, cfg *config.Config) (*Node, error) {
	privKey, err := LoadOrCreateIdentity(cfg.IdentityKeyPath)
	if err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}

	listenAddrs, err := parseMultiaddrs(cfg.ListenAddrs)
	if err != nil {
		return nil, fmt.Errorf("listen addrs: %w", err)
	}

	opts := []libp2p.Option{
		libp2p.Identity(privKey),
		libp2p.ListenAddrs(listenAddrs...),
		libp2p.NATPortMap(),
		libp2p.EnableHolePunching(),
		libp2p.EnableNATService(),
	}

	psk, err := loadSwarmKey(cfg.SwarmKeyPath)
	if err != nil {
		if os.Getenv("LIBP2P_FORCE_PNET") == "1" {
			return nil, fmt.Errorf("swarm key required (LIBP2P_FORCE_PNET=1): %w", err)
		}
		log.Printf("WARNING: No swarm key found at %s -- running without private network", cfg.SwarmKeyPath)
	} else {
		opts = append(opts, libp2p.PrivateNetwork(psk))
	}

	bootstrapInfos := parseBootstrapPeers(cfg.BootstrapPeers)
	if len(bootstrapInfos) > 0 {
		opts = append(opts,
			libp2p.EnableAutoRelayWithStaticRelays(bootstrapInfos),
			libp2p.EnableRelayService(),
		)
	}

	cm, _ := connmgr.NewConnManager(10, 50)
	opts = append(opts, libp2p.ConnectionManager(cm))

	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("create host: %w", err)
	}

	dhtMode := dht.ModeAutoServer
	if cfg.BootstrapMode {
		dhtMode = dht.ModeServer
	}

	var dhtOpts []dht.Option
	dhtOpts = append(dhtOpts, dht.Mode(dhtMode))
	if len(bootstrapInfos) > 0 {
		dhtOpts = append(dhtOpts, dht.BootstrapPeers(bootstrapInfos...))
	}

	kdht, err := dht.New(ctx, h, dhtOpts...)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("create DHT: %w", err)
	}

	if err := kdht.Bootstrap(ctx); err != nil {
		kdht.Close()
		h.Close()
		return nil, fmt.Errorf("bootstrap DHT: %w", err)
	}

	for _, pi := range bootstrapInfos {
		go func(pi peer.AddrInfo) {
			if err := h.Connect(ctx, pi); err != nil {
				log.Printf("Bootstrap peer %s: %v", pi.ID.String()[:16], err)
			} else {
				log.Printf("Connected to bootstrap peer %s", pi.ID.String()[:16])
			}
		}(pi)
	}

	return &Node{Host: h, DHT: kdht, Config: cfg}, nil
}

func (n *Node) Peers() []peer.ID {
	return n.Host.Network().Peers()
}

func (n *Node) PeerID() peer.ID {
	return n.Host.ID()
}

func (n *Node) Addrs() []ma.Multiaddr {
	return n.Host.Addrs()
}

func (n *Node) IsConnected(pid peer.ID) bool {
	return n.Host.Network().Connectedness(pid) == network.Connected
}

func (n *Node) Close() error {
	if err := n.DHT.Close(); err != nil {
		log.Printf("DHT close: %v", err)
	}
	return n.Host.Close()
}

func loadSwarmKey(path string) (pnet.PSK, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return pnet.DecodeV1PSK(f)
}

func parseMultiaddrs(addrs []string) ([]ma.Multiaddr, error) {
	result := make([]ma.Multiaddr, 0, len(addrs))
	for _, a := range addrs {
		maddr, err := ma.NewMultiaddr(a)
		if err != nil {
			return nil, fmt.Errorf("invalid multiaddr %q: %w", a, err)
		}
		result = append(result, maddr)
	}
	return result, nil
}

func parseBootstrapPeers(addrs []string) []peer.AddrInfo {
	var infos []peer.AddrInfo
	for _, a := range addrs {
		maddr, err := ma.NewMultiaddr(a)
		if err != nil {
			log.Printf("Invalid bootstrap addr %q: %v", a, err)
			continue
		}
		pi, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Printf("Invalid bootstrap peer %q: %v", a, err)
			continue
		}
		infos = append(infos, *pi)
	}
	return infos
}
