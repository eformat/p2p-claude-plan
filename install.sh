#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="p2p-claude-plans"
INSTALL_DIR="${HOME}/.local/bin"
SKILL_DIR="${HOME}/.claude/skills/check-team-plans"
SERVICE_DIR="${HOME}/.config/systemd/user"

echo "Building ${BINARY}..."
cd "${SCRIPT_DIR}"
go build -o "${BINARY}" ./cmd/p2p-claude-plans/

echo "Installing binary to ${INSTALL_DIR}/"
mkdir -p "${INSTALL_DIR}"
cp "${BINARY}" "${INSTALL_DIR}/"

echo "Installing skill to ${SKILL_DIR}/"
mkdir -p "${SKILL_DIR}"
cp skill/SKILL.md "${SKILL_DIR}/SKILL.md"

echo "Installing systemd user service..."
mkdir -p "${SERVICE_DIR}"
cp p2p-claude-plans.service "${SERVICE_DIR}/"
systemctl --user daemon-reload

echo ""
echo "Installation complete."
echo ""
echo "Next steps:"
echo ""
echo "  1. Generate a swarm key (one person does this, shares with team):"
echo "     ${BINARY} keygen > ~/.claude/p2p-plans.key"
echo ""
echo "  2. Create config file ~/.claude/p2p-plans.yaml:"
echo "     cp ${SCRIPT_DIR}/config.example.yaml ~/.claude/p2p-plans.yaml"
echo "     # Edit bootstrap_peers with your team's bootstrap node address"
echo ""
echo "  3. Start the daemon:"
echo "     systemctl --user enable --now p2p-claude-plans"
echo ""
echo "  4. In Claude Code, use /check-team-plans to see teammates' plans"
echo ""
echo "Firewall (if needed):"
echo "  sudo firewall-cmd --add-port=4001/tcp   # or your chosen port"
