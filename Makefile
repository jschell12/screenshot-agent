.PHONY: install start build daemon daemon-stop daemon-restart daemon-status daemon-log

INSTALL_DIR := $(HOME)/.local/bin
LAUNCHD_LABEL := com.xmuggle.daemon

install: build
	install -d $(INSTALL_DIR)
	install -m 0755 xmuggled $(INSTALL_DIR)/xmuggled
	launchctl kill SIGTERM gui/$(shell id -u)/$(LAUNCHD_LABEL) 2>/dev/null || true

start:
	npm start

build:
	go build -o xmuggled ./cmd/xmuggled/

daemon: build
	./xmuggled start

daemon-stop:
	./xmuggled stop

daemon-restart:
	launchctl kill SIGTERM gui/$(shell id -u)/$(LAUNCHD_LABEL)

daemon-status:
	./xmuggled status

daemon-log:
	./xmuggled log 50
