---
name: screenshot-fix
description: >-
  Analyze a screenshot to identify a bug or UI issue, then fix the code in the
  target repo. Optionally forward to a remote machine for processing.
---

# Screenshot Fix

Give this tool a screenshot (and optionally a message describing what's wrong), and it spawns a Claude Code agent to analyze the image, find the relevant code, fix it, push a branch, create a PR, and merge it.

## When to trigger

- User provides a screenshot and mentions a repo that needs changes
- User says "fix this screenshot", "screenshot fix", "fix this"
- User invokes `/screenshot-fix`
- User drops an image and describes a bug or desired change

## Steps

1. Gather the required information:
   - **Screenshot path**: The image file to analyze. If the user references a screenshot on their Desktop, find it (e.g., the latest `Screenshot*.png` on `~/Desktop`). If they pasted an image, save it to `/tmp/screenshot-fix-<timestamp>.png` first.
   - **Repo**: GitHub repo (`owner/name` or URL) or local path. Ask if not provided.
   - **Message** (optional): Additional context about what to fix — what's wrong, what to look for, what the expected behavior should be.

2. Decide the mode:
   - **Local** (default): Process on this machine. No extra setup needed.
   - **Remote** (`--remote`): Forward to a remote machine's daemon for processing. Requires SSH setup (`make setup`).

3. Run the CLI:

```bash
# Local — process right here
screenshot-agent <screenshot-path> --repo <repo> --msg "<message>"

# Remote — forward to another machine
screenshot-agent <screenshot-path> --repo <repo> --msg "<message>" --remote
```

4. Report the result to the user. If successful, mention what was fixed and that they can `git pull` to get the changes.

## Prerequisites

- `screenshot-agent` CLI on PATH (install: `pnpm build && pnpm link --global` from the screenshot-agent repo)
- `claude` CLI on PATH
- `gh` CLI authenticated for PR creation/merging
- For remote mode only: SSH configured (`make setup`) + daemon running on remote machine (`make daemon-start`)
