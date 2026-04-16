---
name: look
description: Analyze screenshot(s) to identify bugs or UI issues and fix the code
---

# /look (Cursor Command)

Screenshots are auto-detected from Desktop/Downloads.

## Usage

```bash
# Latest screenshot, local
screenshot-agent --repo <repo> --msg "<message>"

# Specific images
screenshot-agent --repo <repo> --img "<name>" --msg "<message>"

# All unprocessed
screenshot-agent --repo <repo> --all --msg "<message>"

# Forward to another Mac on the LAN
screenshot-agent --repo <repo> --remote --msg "<message>"
```
