---
name: check-team-plans
description: >
  Check what your teammates are working on by querying the P2P plan sharing
  network. Shows plan summaries from all connected peers. Use when the user
  wants to see team plans, check what others are working on, or download a
  specific teammate's plan. Triggers on: "team plans", "check team",
  "what are others working on", "peer plans".
---

# Check Team Plans

Query the P2P plan sharing network to see what teammates are working on.

## Prerequisites

The `p2p-claude-plans` daemon must be running. Check with:

```bash
curl -sf http://localhost:7856/health
```

If it fails, tell the user to start the daemon:
```
p2p-claude-plans &
# or: systemctl --user start p2p-claude-plans
```

## Workflow

### Step 1: Health check

```bash
curl -sf http://localhost:7856/health | jq .
```

Verify `status` is `ok`. Note `peer_count` -- if 0, no teammates are connected yet.

### Step 2: Fetch all plans

```bash
curl -sf http://localhost:7856/plans | jq .
```

This returns a JSON array, one entry per peer. Each has:
- `peer_name`: teammate's display name
- `peer_id`: their libp2p peer ID
- `is_local`: true for the user's own plans (skip these in display)
- `plans`: array of `{id, summary, modified}`
- `error`: set if that peer was unreachable

### Step 3: Display results

Format output as a table grouped by peer. Only show remote peers (where `is_local` is false):

```
## Team Plans

### Alice (12D3KooW...abc)
| # | Plan | Last Modified |
|---|------|---------------|
| 1 | Add AuthBridge fix job to Helm chart | 2026-05-20 |
| 2 | GPU Test Workloads Helm Chart | 2026-05-19 |

### Bob (12D3KooW...def)
| # | Plan | Last Modified |
|---|------|---------------|
| 1 | Graceful ClusterQueue Deletion | 2026-05-18 |
```

If `peer_count` is 0, say: "No teammates connected. Make sure the p2p-claude-plans daemon is running and at least one peer is online."

If any peer has an `error` field, note it: "Could not reach peer X: (error)"

### Step 4: Offer to download

After showing summaries, ask: "Would you like to see the full content of any plan?"

If yes, fetch it:

```bash
curl -sf "http://localhost:7856/plans/{peer_id}/{plan_id}" | jq -r .content
```

Display the full markdown. Offer to save it to `~/.claude/plans/` if the user wants a local copy.

### Filtering

If the user provides arguments (e.g., `/check-team-plans alice`), filter results to only show
plans from peers whose `peer_name` contains the argument (case-insensitive).
