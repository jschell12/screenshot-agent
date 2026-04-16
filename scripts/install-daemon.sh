#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLIST_LABEL="com.look.daemon"
PLIST_DEST="$HOME/Library/LaunchAgents/${PLIST_LABEL}.plist"
PLIST_TEMPLATE="$REPO_DIR/launchd/${PLIST_LABEL}.plist"

echo "=== Installing look daemon ==="
echo ""
echo "This machine will process screenshot tasks pushed into ~/.look/queue/"
echo "by other Macs on the LAN, plus any received via an encrypted git queue."
echo ""

mkdir -p ~/.look/{queue,results,logs}

# Make sure the CLI + daemon binaries are installed
bash "$REPO_DIR/scripts/install-skill.sh"

# Locate lookd (freshly installed)
DAEMON_BIN="$(command -v lookd)"
if [[ -z "$DAEMON_BIN" ]]; then
  echo "Error: lookd not found on PATH after install" >&2
  exit 1
fi

# Generate plist
sed \
  -e "s|__DAEMON_BIN__|${DAEMON_BIN}|g" \
  -e "s|__HOME__|${HOME}|g" \
  "$PLIST_TEMPLATE" > "$PLIST_DEST"

echo "Plist installed at $PLIST_DEST"

launchctl unload "$PLIST_DEST" 2>/dev/null || true
launchctl load "$PLIST_DEST"

echo "Daemon started."
echo ""
echo "Verify:  launchctl list | grep com.look"
echo "Logs:    make daemon-logs"
