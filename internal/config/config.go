package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SwarmKeyPath    string   `yaml:"swarm_key_path"`
	BootstrapPeers  []string `yaml:"bootstrap_peers"`
	PlansDir        string   `yaml:"plans_dir"`
	HTTPPort        int      `yaml:"http_port"`
	ListenAddrs     []string `yaml:"listen_addrs"`
	PeerName        string   `yaml:"peer_name"`
	IdentityKeyPath string   `yaml:"identity_key_path"`
	RequestTimeout  int      `yaml:"request_timeout"`
	BootstrapMode   bool     `yaml:"bootstrap_mode"`
}

func defaults() *Config {
	home, _ := os.UserHomeDir()
	hostname, _ := os.Hostname()
	return &Config{
		SwarmKeyPath:    filepath.Join(home, ".claude", "p2p-plans.key"),
		PlansDir:        filepath.Join(home, ".claude", "plans"),
		HTTPPort:        7856,
		ListenAddrs:     []string{"/ip4/0.0.0.0/tcp/0"},
		PeerName:        hostname,
		IdentityKeyPath: filepath.Join(home, ".claude", "p2p-plans-identity.key"),
		RequestTimeout:  5,
	}
}

func Load(configPath string) (*Config, error) {
	cfg := defaults()

	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".claude", "p2p-plans.yaml")
	}

	if data, err := os.ReadFile(configPath); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	applyEnv(cfg)

	cfg.SwarmKeyPath = expandHome(cfg.SwarmKeyPath)
	cfg.PlansDir = expandHome(cfg.PlansDir)
	cfg.IdentityKeyPath = expandHome(cfg.IdentityKeyPath)

	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("P2P_PLANS_SWARM_KEY"); v != "" {
		cfg.SwarmKeyPath = v
	}
	if v := os.Getenv("P2P_PLANS_BOOTSTRAP"); v != "" {
		cfg.BootstrapPeers = strings.Split(v, ",")
	}
	if v := os.Getenv("P2P_PLANS_DIR"); v != "" {
		cfg.PlansDir = v
	}
	if v := os.Getenv("P2P_PLANS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = port
		}
	}
	if v := os.Getenv("P2P_PLANS_PEER_NAME"); v != "" {
		cfg.PeerName = v
	}
	if v := os.Getenv("P2P_PLANS_LISTEN"); v != "" {
		cfg.ListenAddrs = strings.Split(v, ",")
	}
	if v := os.Getenv("P2P_PLANS_IDENTITY_KEY"); v != "" {
		cfg.IdentityKeyPath = v
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
