BINARY := p2p-claude-plans
INSTALL_DIR := $(HOME)/.local/bin
SKILL_DIR := $(HOME)/.claude/skills/check-team-plans

.PHONY: build test install uninstall run keygen clean logs status

build:
	go build -o $(BINARY) ./cmd/p2p-claude-plans/

test:
	go test -v ./...

install:
	bash install.sh

uninstall:
	systemctl --user stop p2p-claude-plans 2>/dev/null || true
	systemctl --user disable p2p-claude-plans 2>/dev/null || true
	rm -f $(INSTALL_DIR)/$(BINARY)
	rm -rf $(SKILL_DIR)
	rm -f $(HOME)/.config/systemd/user/p2p-claude-plans.service
	systemctl --user daemon-reload

run: build
	./$(BINARY)

keygen:
	@$(INSTALL_DIR)/$(BINARY) keygen

clean:
	rm -f $(BINARY)

logs:
	journalctl --user -u p2p-claude-plans -f

status:
	@curl -sf http://localhost:7856/health | python3 -m json.tool
