.PHONY: build install daemon-install daemon-uninstall daemon-start daemon-stop daemon-logs link clean

DAEMON_PLIST := $(HOME)/Library/LaunchAgents/com.look.daemon.plist
INSTALL_PREFIX ?= /usr/local

build:
	@mkdir -p bin
	go build -o bin/look ./cmd/look
	go build -o bin/lookd ./cmd/lookd

# Install binaries + /look skill for Claude/Cursor
install: build
	bash scripts/install-skill.sh

# Install the queue-processing daemon (launchd)
daemon-install: build
	bash scripts/install-daemon.sh

daemon-uninstall:
	launchctl unload $(DAEMON_PLIST) 2>/dev/null || true
	rm -f $(DAEMON_PLIST)

daemon-start:
	launchctl load $(DAEMON_PLIST)

daemon-stop:
	launchctl unload $(DAEMON_PLIST)

daemon-logs:
	tail -f ~/.look/logs/daemon.stdout.log

# Interactive LAN discovery + tunnel/push/pull
link:
	bash scripts/mac-link.sh

clean:
	rm -rf bin
