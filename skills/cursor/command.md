---
name: screenshot-fix
description: Analyze a screenshot to identify a bug or UI issue, then fix the code in the target repo
---

# Screenshot Fix (Cursor Command)

Give this tool a screenshot + optional message, and it fixes the code.

## Usage

Gather:
1. **Screenshot**: path to the image file
2. **Repo**: GitHub repo (owner/name) or local path
3. **Message** (optional): what to fix or look for

Run in the terminal:

```bash
# Local processing
screenshot-agent <screenshot> --repo <repo> --msg "<message>"

# Remote processing (forward to another machine)
screenshot-agent <screenshot> --repo <repo> --msg "<message>" --remote
```

The agent analyzes the screenshot, finds relevant code, makes fixes, creates a PR, and merges it.
