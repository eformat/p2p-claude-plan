# p2p-claude-plans

Share Claude Code plan files with your team over an encrypted P2P network. Each team member runs a lightweight daemon that watches `~/.claude/plans/` and shares plan summaries with peers. Query the network from Claude Code with the `/check-team-plans` skill.

## How it works

```
You (Claude Code)                    Teammate
  |                                    |
  | /check-team-plans                  |
  v                                    |
Local daemon (localhost:7856)          Their daemon
  |                                    |
  |-------- libp2p (encrypted) ------->|
  |<------- plan summaries ------------|
  v
"Alice is working on: Fix OAuth redirect loop"
```

All traffic is encrypted with a shared swarm key (XSalsa20). Only peers with the same key can connect. Peer discovery uses a Kademlia DHT with bootstrap nodes, so it works across the internet -- no VPN required.

## Prerequisites

- Go 1.25+ (required by libp2p dependencies)
- A team member willing to run a bootstrap node (any cheap VPS works)

## Quick start

### 1. Build and install

```bash
git clone <this-repo> && cd p2p-claude-plan
make install
```

This installs the binary to `~/.local/bin/` and the Claude Code skill to `~/.claude/skills/`.

### 2. Generate a swarm key (one person does this)

```bash
p2p-claude-plans keygen > ~/.claude/p2p-plans.key
```

Share this file with your team out-of-band (Slack DM, encrypted email, etc.). Everyone uses the same key.

### 3. Set up the bootstrap node

One team member runs the bootstrap node on a publicly reachable server:

```bash
# On the VPS
scp ~/.claude/p2p-plans.key vps:~/.claude/p2p-plans.key
ssh vps
p2p-claude-plans --bootstrap-mode --listen /ip4/0.0.0.0/tcp/4001
```

It prints a multiaddr like:
```
/ip4/203.0.113.50/tcp/4001/p2p/12D3KooWAbCdEf...
```

Share this multiaddr with the team.

### 4. Configure your node

```bash
cp config.example.yaml ~/.claude/p2p-plans.yaml
```

Edit `~/.claude/p2p-plans.yaml`:

```yaml
swarm_key_path: ~/.claude/p2p-plans.key
peer_name: your-name
bootstrap_peers:
  - /ip4/203.0.113.50/tcp/4001/p2p/12D3KooWAbCdEf...
```

### 5. Start the daemon

```bash
# As a systemd service (recommended)
systemctl --user enable --now p2p-claude-plans

# Or manually
p2p-claude-plans &
```

### 6. Verify it's working

```bash
# Check daemon health
systemctl --user status p2p-claude-plans

# Quick status
curl -sf http://localhost:7856/health | jq '{status, plan_count, peer_count}'

# Who's online?
curl -sf http://localhost:7856/peers | jq .

# See only teammate plans (not your own)
curl -sf http://localhost:7856/plans | jq '.[] | select(.is_local == false) | {peer_name, plans: [.plans[].summary]}'
```

### 7. Use it

In Claude Code:
```
/check-team-plans
```

## Firewall

If you're behind a firewall, open the libp2p port:

```bash
sudo firewall-cmd --add-port=4001/tcp
```

The daemon also supports NAT traversal (UPnP, relay, hole punching), so many home networks work without manual firewall changes.

## Configuration reference

Config file: `~/.claude/p2p-plans.yaml`

| Field | Default | Description |
|-------|---------|-------------|
| `swarm_key_path` | `~/.claude/p2p-plans.key` | Path to the shared encryption key |
| `plans_dir` | `~/.claude/plans/` | Directory to watch for plan files |
| `http_port` | `7856` | Local HTTP API port |
| `peer_name` | hostname | Your display name shown to teammates |
| `listen_addrs` | `[/ip4/0.0.0.0/tcp/0]` | libp2p listen addresses |
| `bootstrap_peers` | `[]` | Bootstrap node multiaddrs |
| `identity_key_path` | `~/.claude/p2p-plans-identity.key` | Persistent peer identity |
| `request_timeout` | `5` | Seconds timeout for peer requests |
| `bootstrap_mode` | `false` | Run as a DHT server (for bootstrap nodes) |

Environment variable overrides:

| Variable | Maps to |
|----------|---------|
| `P2P_PLANS_SWARM_KEY` | `swarm_key_path` |
| `P2P_PLANS_DIR` | `plans_dir` |
| `P2P_PLANS_PORT` | `http_port` |
| `P2P_PLANS_PEER_NAME` | `peer_name` |
| `P2P_PLANS_LISTEN` | `listen_addrs` (comma-separated) |
| `P2P_PLANS_BOOTSTRAP` | `bootstrap_peers` (comma-separated) |
| `P2P_PLANS_IDENTITY_KEY` | `identity_key_path` |

## HTTP API

The daemon exposes a local-only API on `127.0.0.1:<http_port>`:

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Status, peer count, plan count |
| `GET /plans` | Plans from all connected peers |
| `GET /plans/{peer_id}/{plan_id}` | Full content of a specific plan |
| `GET /peers` | List connected peers |

## Testing your setup

After completing the quick start, run through these checks to verify everything works.

### 1. Is the daemon running?

```bash
curl -sf http://localhost:7856/health | jq .
```

Expected output:
```json
{
  "status": "ok",
  "peer_name": "your-name",
  "peer_id": "12D3KooW...",
  "plan_count": 38,
  "peer_count": 1
}
```

- `plan_count` should match the number of `.md` files in `~/.claude/plans/`
- `peer_count` should be >= 1 if the bootstrap node is reachable

### 2. Can you reach the bootstrap node?

```bash
# TCP connectivity test (should connect, not timeout)
nc -zv <BOOTSTRAP_IP> 4001

# Check from the daemon's perspective
curl -sf http://localhost:7856/peers | jq .
```

Expected: at least one peer with `"connected": true`. If the list is empty, see Troubleshooting below.

### 3. Are your local plans indexed?

```bash
curl -sf http://localhost:7856/plans | jq '.[0].plans[:3]'
```

Expected: a JSON array of your plan summaries with `id`, `summary`, and `modified` fields.

### 4. Can you fetch plans from peers?

```bash
# List plans from all peers (local + remote)
curl -sf http://localhost:7856/plans | jq '.[] | {peer_name, is_local, plan_count: (.plans | length), error}'
```

Each peer shows its name, whether it's local, and how many plans it has. Remote peers with plans means the full P2P exchange is working.

### 5. Download a specific plan

```bash
# Pick a plan_id and peer_id from the /plans output, then:
curl -sf http://localhost:7856/plans/<peer_id>/<plan_id> | jq -r .content | head -20
```

### 6. Test the Claude Code skill

In Claude Code, type:
```
/check-team-plans
```

This should display a formatted table of plans grouped by teammate.

### 7. Test with a second local peer (no VPS needed)

To test plan exchange without a second machine, run two instances locally. Each peer needs its own identity key, port, plan directory, and peer name:

```bash
# Setup
mkdir -p /tmp/alice-plans /tmp/bob-plans
echo "# Redesign the login page" > /tmp/alice-plans/login-redesign.md
echo "# Fix payment webhook retry logic" > /tmp/bob-plans/fix-webhooks.md
```

```bash
# Terminal 1 -- peer "alice"
P2P_PLANS_DIR=/tmp/alice-plans \
P2P_PLANS_PORT=7856 \
P2P_PLANS_PEER_NAME=alice \
P2P_PLANS_IDENTITY_KEY=/tmp/alice-identity.key \
P2P_PLANS_SWARM_KEY=/dev/null \
  p2p-claude-plans --listen /ip4/127.0.0.1/tcp/9001
```

Note the multiaddr printed (e.g., `/ip4/127.0.0.1/tcp/9001/p2p/12D3KooW...`).

```bash
# Terminal 2 -- peer "bob" (use alice's multiaddr as bootstrap)
P2P_PLANS_DIR=/tmp/bob-plans \
P2P_PLANS_PORT=7857 \
P2P_PLANS_PEER_NAME=bob \
P2P_PLANS_IDENTITY_KEY=/tmp/bob-identity.key \
P2P_PLANS_SWARM_KEY=/dev/null \
P2P_PLANS_BOOTSTRAP=/ip4/127.0.0.1/tcp/9001/p2p/12D3KooW... \
  p2p-claude-plans --listen /ip4/127.0.0.1/tcp/9002
```

Wait a few seconds for the DHT to connect, then test from bob's API:

```bash
# Should show both alice's and bob's plans
curl -sf http://localhost:7857/plans | jq '.[] | {peer_name, is_local, plans}'
```

Expected: bob sees alice's "Redesign the login page" as a remote peer, and his own "Fix payment webhook retry logic" locally. Query alice's API too:

```bash
curl -sf http://localhost:7856/plans | jq '.[] | {peer_name, is_local, plans}'
```

**Important:** Each peer must have a unique `P2P_PLANS_IDENTITY_KEY` path. If two peers share the same identity key, they have the same peer ID and cannot connect to each other.

Cleanup:
```bash
rm -rf /tmp/alice-plans /tmp/bob-plans /tmp/alice-identity.key /tmp/bob-identity.key
```

### Quick health check (one-liner)

```bash
curl -sf http://localhost:7856/health | jq '{status, plan_count, peer_count}'
```

## Troubleshooting

**"No teammates connected" / peer_count is 0**
- Check TCP connectivity to bootstrap: `nc -zv <BOOTSTRAP_IP> 4001`
- If "connection refused": the daemon isn't running on the VPS. Check with `ssh vps "systemctl status p2p-claude-plans"`
- If timeout: the port isn't open. Check AWS Security Group (inbound TCP 4001) and OS firewall
- Check that both peers have the exact same swarm key file
- View local peer list: `curl -sf http://localhost:7856/peers | jq .`

**"Connection refused" on localhost:7856**
- The local daemon isn't running: `systemctl --user status p2p-claude-plans`
- Check logs: `journalctl --user -u p2p-claude-plans -n 20`
- Port conflict: `lsof -i :7856`

**Peers connected but plans don't load**
- Check the request timeout (default 5s) in config
- Verify the remote peer's `~/.claude/plans/` has `.md` files
- Test directly: `curl -sf http://localhost:7856/plans | jq '.[] | {peer_name, error}'`

**Swarm key mismatch**
- Connections will silently fail (timeout, not error). Ensure all peers use the exact same key file
- Verify: `md5sum ~/.claude/p2p-plans.key` should match across all machines
- Re-share the key if in doubt

**Bootstrap node setup on AWS (systemd)**
```bash
# Copy binary and key to VPS
scp p2p-claude-plans vps:~/
scp ~/.claude/p2p-plans.key vps:~/.claude/p2p-plans.key

# On the VPS: install to /usr/local/bin (avoids home dir permission issues with systemd)
ssh vps "sudo cp ~/p2p-claude-plans /usr/local/bin/ && sudo chmod 755 /usr/local/bin/p2p-claude-plans"

# Create systemd unit
ssh vps "sudo tee /etc/systemd/system/p2p-claude-plans.service > /dev/null << 'EOF'
[Unit]
Description=P2P Claude Plans Bootstrap Node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ec2-user
ExecStart=/usr/local/bin/p2p-claude-plans --bootstrap-mode --listen /ip4/0.0.0.0/tcp/4001
Restart=on-failure
RestartSec=10
Environment=LIBP2P_FORCE_PNET=1

[Install]
WantedBy=multi-user.target
EOF"

# Enable and start
ssh vps "sudo systemctl daemon-reload && sudo systemctl enable --now p2p-claude-plans"

# Verify
ssh vps "sudo journalctl -u p2p-claude-plans -n 5 --no-pager"
```

## Makefile targets

```
make build       # Build the binary
make test        # Run all tests (security, path traversal, functionality)
make install     # Build + install binary, skill, and systemd service
make uninstall   # Remove everything
make run         # Build and run locally
make keygen      # Generate a new swarm key
make logs        # Follow systemd logs
make status      # Check daemon health
make clean       # Remove build artifacts
```
