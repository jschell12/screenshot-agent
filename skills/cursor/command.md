---
name: xmuggle
description: Analyze screenshot(s) to identify bugs or UI issues and fix the code
---

# /xmuggle (Cursor Command)

Screenshots are auto-detected from ~/Desktop.

## Usage

```bash
# Latest screenshot, local
xmuggle --repo <repo> --msg "<message>"

# Specific images
xmuggle --repo <repo> --img "<name>" --msg "<message>"

# All unprocessed
xmuggle --repo <repo> --all --msg "<message>"

# Forward to another Mac on the LAN
xmuggle --repo <repo> --remote --msg "<message>"
```

## First-time pairing (AI-assisted)

When setting up `--remote --git` on a new machine, use `--json` to fetch available peers and ask the user conversationally which to pair with, then re-run with `--peer`:

```bash
# Step 1: base setup + list peers (JSON — no prompt)
xmuggle init-send <owner/repo> --json
# or for the receiver side:
xmuggle init-recv <owner/repo> --json

# Step 2: after user picks one
xmuggle init-send <owner/repo> --peer <chosen-host>
```
