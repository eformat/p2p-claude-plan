# Architecture

## System overview

```
                         ┌─────────────────────────────┐
                         │     Claude Code (user)       │
                         │                              │
                         │  /check-team-plans           │
                         │    └─ curl localhost:7856     │
                         └──────────┬───────────────────┘
                                    │
                         ┌──────────▼───────────────────┐
                         │  Local HTTP API               │
                         │  (127.0.0.1:7856)             │
                         │                               │
                         │  /plans  /peers  /health      │
                         └──────────┬───────────────────┘
                                    │
                    ┌───────────────┼───────────────┐
                    │               │               │
              ┌─────▼─────┐  ┌─────▼─────┐  ┌─────▼─────┐
              │ PlanStore  │  │  libp2p   │  │  libp2p   │
              │ (local fs) │  │  Peer A   │  │  Peer B   │
              │            │  │           │  │           │
              │ ~/.claude/ │  │  Alice's  │  │  Bob's    │
              │  plans/    │  │  daemon   │  │  daemon   │
              └────────────┘  └───────────┘  └───────────┘
```

The daemon is a single Go binary with two server loops:
1. **libp2p host** -- encrypted P2P connections to teammates
2. **HTTP server** -- local-only API for the Claude Code skill

## Private network (swarm key)

All peer connections are encrypted using a pre-shared key (PSK). The PSK file format is multicodec:

```
/key/swarm/psk/1.0.0/
/base16/
<64 hex characters = 32 bytes>
```

The key is loaded at startup via `pnet.DecodeV1PSK()` and passed to `libp2p.PrivateNetwork(psk)`. libp2p wraps every TCP connection with XSalsa20 stream encryption using this key before the Noise handshake even begins. This means:

- Peers without the key cannot complete the TCP handshake
- The DHT routing table only contains peers from the private network
- There is no accidental cross-talk with public libp2p networks

**Transport restriction:** Only TCP works with PSK. QUIC and WebRTC have their own encryption layers that conflict with the PSK wrapper. The node explicitly uses `libp2p.NoTransports` + TCP when PSK is active.

**LIBP2P_FORCE_PNET=1:** Setting this env var makes libp2p refuse to start without a PSK. The systemd unit sets this by default to prevent accidentally running without encryption.

## Peer discovery

### Kademlia DHT

Peers discover each other using a Kademlia Distributed Hash Table (DHT). The DHT runs entirely within the private network -- only peers that pass the PSK handshake appear in the routing table.

### Bootstrap flow

```
                    ┌────────────────────┐
                    │  Bootstrap Node    │
                    │  (public VPS)      │
                    │  DHT Mode: Server  │
                    └─────┬──────┬───────┘
                          │      │
           ┌──────────────┘      └──────────────┐
           │                                     │
     ┌─────▼─────┐                         ┌─────▼─────┐
     │  Peer A   │                         │  Peer B   │
     │  (home)   │                         │  (office) │
     └─────┬─────┘                         └─────┬─────┘
           │                                     │
           └──────── DHT discovers ──────────────┘
                     direct connection
```

1. On startup, each peer connects to the bootstrap node explicitly
2. `kdht.Bootstrap()` starts periodic DHT refresh (every ~2 minutes)
3. DHT lookups propagate peer addresses through the routing table
4. After a few rounds, peers know each other's addresses directly
5. If the bootstrap node goes down, existing peers remain connected; new peers cannot join until it returns

### DHT modes

- **ModeAutoServer** (default): Acts as DHT server when publicly reachable, client when behind NAT
- **ModeServer** (bootstrap node): Always responds to DHT queries, critical for the network

## NAT traversal

Most developers are behind NAT (home routers, corporate firewalls). The daemon uses three layers:

### Layer 1: UPnP / NAT-PMP

`libp2p.NATPortMap()` automatically requests port forwarding from the local router. Works with many consumer routers. If successful, the peer gets a publicly routable address.

### Layer 2: AutoRelay

If UPnP fails, AutoNAT detects the peer is behind NAT. AutoRelay then advertises a relay address through the bootstrap node:

```
/ip4/<bootstrap-ip>/tcp/4001/p2p/<bootstrap-id>/p2p-circuit/p2p/<my-id>
```

Traffic flows: Peer A -> Bootstrap (relay) -> Peer B. Circuit Relay v2 has resource limits to prevent abuse.

### Layer 3: Hole Punching (DCUtR)

Once two NAT'd peers are connected via relay, DCUtR (Direct Connection Upgrade through Relay) coordinates simultaneous TCP connection attempts to punch through both NATs. If successful, the relay is dropped and traffic flows directly.

```
Before:  A ──relay──> Bootstrap ──relay──> B
After:   A ──────── direct ────────────> B
```

## Custom protocol

### Protocol ID

`/claude-plans/1.0.0`

Registered via `host.SetStreamHandler()`. Each request opens one libp2p stream, exchanges one request and one response, then closes the stream.

### Wire format

Length-prefixed JSON. Each message is a 4-byte big-endian uint32 (payload length) followed by the JSON payload. Max message size: 10MB.

```
┌──────────┬──────────────────────┐
│ 4 bytes  │  N bytes             │
│ uint32BE │  JSON payload        │
│ (length) │                      │
└──────────┴──────────────────────┘
```

### Request types

**List plans:**
```json
{"type": "list"}
```

Response:
```json
{
  "peer_name": "alice",
  "peer_id": "12D3KooW...",
  "plans": [
    {"id": "fix-auth-flow", "summary": "Fix OAuth redirect loop", "modified": "2026-05-20T10:00:00Z"}
  ]
}
```

**Get plan content:**
```json
{"type": "get", "plan_id": "fix-auth-flow"}
```

Response:
```json
{
  "peer_name": "alice",
  "peer_id": "12D3KooW...",
  "plan_id": "fix-auth-flow",
  "summary": "Fix OAuth redirect loop",
  "content": "# Fix OAuth redirect loop\n\n## Context\n..."
}
```

## Plan store

### File watching

The store uses `fsnotify` to watch `~/.claude/plans/` for create, write, and delete events. On startup, it does a full directory scan to populate the in-memory index.

### Summary extraction

Every plan file starts with a markdown H1 heading. The extractor:
1. Reads the first non-empty line
2. If it starts with `# `, strips the prefix
3. If the result starts with `Plan: `, strips that too
4. Falls back to humanizing the filename

### Caching

- **Summaries**: cached in memory, updated on fsnotify events
- **Full content**: read from disk on demand (not cached)

This keeps memory low while providing fast summary listing.

## HTTP API

Bound to `127.0.0.1` only -- never accessible from the network.

### GET /plans

Aggregates plans from the local store and all connected peers. The fan-out is concurrent (`sync.WaitGroup` + goroutine per peer) with a per-peer timeout (default 5s). Unreachable peers are reported in the `error` field, not blocking the response.

### GET /plans/{peer_id}/{plan_id}

If `peer_id` matches the local node, reads from disk. Otherwise, opens a libp2p stream to the remote peer and proxies the response. The skill only ever talks to localhost.

### GET /peers

Returns all peers from the libp2p network with their addresses and connection status.

### GET /health

Returns status, peer name, peer ID, plan count, and connected peer count.

## Security model

### What's protected

- **Transport encryption**: All peer traffic encrypted with shared PSK (XSalsa20) + Noise handshake
- **Network isolation**: Only peers with the swarm key can join the DHT
- **Local API**: HTTP bound to 127.0.0.1, inaccessible from the network
- **Input validation**: Plan IDs validated against `[a-zA-Z0-9_-.]`, rejecting path traversal

### What's not protected

- **Plan content**: Anyone with the swarm key can read all plans. This is by design -- it's a team sharing tool
- **Authentication**: No per-user auth beyond the shared key. If the key leaks, regenerate and redistribute
- **Integrity**: No signing of plan content. A compromised peer could serve modified plans

### Threat boundaries

- The swarm key is the trust boundary. Protect it like a shared password
- The bootstrap node sees relay traffic but cannot decrypt it (PSK encryption happens at the transport layer, before relay)
- Plan files may contain sensitive information (architecture decisions, security fixes). Consider this before sharing

## Code structure

```
cmd/p2p-claude-plans/main.go      Entry point, CLI flags, component wiring
internal/
  config/config.go                YAML + env + flag config loading
  node/
    node.go                       libp2p host, DHT, NAT traversal
    keygen.go                     Swarm key and identity key generation
  protocol/
    messages.go                   JSON request/response types
    handler.go                    Incoming stream handler
    client.go                     Outgoing stream queries
  planstore/store.go              fsnotify watcher, summary extraction
  api/
    server.go                     HTTP server lifecycle
    handlers.go                   Route handlers with peer fan-out
```

### Dependency graph

```
main.go
  ├── config
  ├── planstore
  ├── node (depends on: config)
  ├── protocol (depends on: planstore, node)
  └── api (depends on: planstore, node, protocol)
```
