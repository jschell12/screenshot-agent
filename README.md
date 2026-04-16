# screenshot-agent

Screenshot-driven code fixes. Give it an image, a target repo, and an optional message — a Claude Code agent analyzes the screenshot, finds the relevant code, fixes it, opens a PR, and merges it.

## Quick start

```bash
git clone git@github.com:jschell12/screenshot-agent.git
cd screenshot-agent
pnpm install && pnpm build
pnpm link --global
```

## Usage

```bash
# Fix a bug shown in a screenshot
screenshot-agent ./bug.png --repo jschell12/my-app

# With context about what to fix
screenshot-agent ./error.png --repo jschell12/my-app --msg "the submit button overlaps the footer on mobile"

# Forward to a remote machine for processing
screenshot-agent ./bug.png --repo jschell12/my-app --remote --msg "fix this login error"
```

### What happens

1. Agent reads the screenshot (Claude sees images natively)
2. Analyzes what's wrong, using your message for context
3. Clones the repo, creates a branch
4. Finds and fixes the relevant code
5. Pushes, opens a PR, and merges it

## Modes

### Local (default)

Processes on the current machine. Just needs `claude` and `gh` on PATH.

```bash
screenshot-agent ./bug.png --repo owner/repo --msg "description"
```

### Remote (`--remote`)

Forwards the screenshot + task to another machine over SSH. A daemon on that machine picks it up and processes it via [agent-queue](https://github.com/jschell12/agent-queue) with parallel workers and merge locking.

```bash
screenshot-agent ./bug.png --repo owner/repo --msg "description" --remote
```

### Drop folder (auto-sync)

On machines with the watcher installed (`make i-wm`), drop an image into `~/Desktop/<remote-ip>/` or `~/Downloads/<remote-ip>/` and it auto-syncs to the remote daemon.

Specify repo/message with a sidecar JSON:
```
~/Desktop/192.168.1.100/bug.png
~/Desktop/192.168.1.100/bug.json  →  {"repo": "owner/repo", "msg": "fix the button"}
```

Or use subdirectories:
```
~/Desktop/192.168.1.100/owner/repo/bug.png
```

## Setup

### Any machine (local use only)

```bash
pnpm install && pnpm build && pnpm link --global
```

That's it. Run `screenshot-agent` anywhere.

### Remote processing (two machines)

```bash
# Both machines: configure SSH to each other
make setup

# Machine that processes (personal laptop, CI box, etc.)
make i-pm        # installs daemon via launchd

# Machine that sends (work laptop, any other machine)
make i-wm        # installs CLI, watcher, Claude/Cursor skills
```

## Daemon

The daemon watches `~/.screenshot-agent/queue/` for tasks, adds them to agent-queue, and spawns up to 3 parallel Claude workers with merge locking.

```bash
make daemon-start    # start
make daemon-stop     # stop
make daemon-logs     # tail logs
```

Environment variables:
- `MAX_WORKERS` — max parallel workers per project (default: 3)
- `POLL_INTERVAL_MS` — queue check interval in ms (default: 5000)
- `AQ_SCRIPTS` — path to agent-queue scripts directory

## How it works

```
screenshot + repo + message
        │
        ├── local mode ────► claude agent spawns directly
        │                      reads image → fixes code → PR → merge
        │
        └── remote mode ───► rsync to remote machine
                                    │
                              daemon adds to agent-queue
                                    │
                              workers claim tasks:
                                agent-1 ──► claim → fix → agent-merge (locked) → complete
                                agent-2 ──► claim → fix → agent-merge (locked) → complete
                                agent-3 ──► claim → fix → agent-merge (locked) → complete
                                    │
                              result written ──► work machine polls ──► git pull
```

## Requirements

- Node.js 22+
- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code)
- [GitHub CLI](https://cli.github.com/) (`gh`) authenticated
- [agent-queue](https://github.com/jschell12/agent-queue) (for daemon/remote mode)
