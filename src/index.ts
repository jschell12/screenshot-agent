import { resolve } from "node:path";
import { existsSync, mkdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { spawn } from "node:child_process";
import { buildPrompt } from "./prompt.js";
import { spawnAgent } from "./spawn.js";
import { loadConfig } from "./config.js";
import { createTaskId, writeTask, type TaskPayload } from "./queue.js";
import { sendTask, pollForResult } from "./remote.js";
import {
  findLatestImage,
  findImageByName,
  ingestFromScanDirs,
  markProcessed,
  listUnprocessed,
  listAllImages,
} from "./images.js";

const USAGE = `Usage: screenshot-agent --repo <repo> [--img <name>]... [--msg "context"] [--remote] [--list] [--scan]

  --repo <repo>  GitHub repo (owner/name or URL) or local path
  --img  <name>  Select image(s) by name or partial match (repeatable)
                 If omitted, uses the latest unprocessed image
  --msg  <msg>   Optional context to guide the agent
  --remote       Send to remote machine for processing (requires 'make setup')
  --list         List all images in ~/.screenshot-agent/ and their status
  --scan         Scan ~/Desktop and ~/Downloads, move images to ~/.screenshot-agent/

Image store:
  Images live in ~/.screenshot-agent/. Processed images tracked in .tracked.
  Use --scan to ingest from Desktop/Downloads, or copy images there manually.

Examples:
  screenshot-agent --scan                                         # ingest from Desktop/Downloads
  screenshot-agent --list                                         # see all images + status
  screenshot-agent --repo jschell12/my-app                        # latest unprocessed image
  screenshot-agent --repo jschell12/my-app --msg "fix the btn"    # with context
  screenshot-agent --repo jschell12/my-app --img "Screenshot 2026-04-14"  # specific image
  screenshot-agent --repo jschell12/my-app --img bug1.png --img bug2.png  # multiple images`;

function parseArgs(argv: string[]) {
  const args = argv.slice(2);

  if (args.includes("--help") || args.includes("-h")) {
    console.log(USAGE);
    process.exit(0);
  }

  let repo: string | undefined;
  let message: string | undefined;
  let remote = false;
  let list = false;
  let scan = false;
  const imgs: string[] = [];

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === "--repo" && i + 1 < args.length) {
      repo = args[++i];
    } else if (arg === "--msg" && i + 1 < args.length) {
      message = args[++i];
    } else if (arg === "--img" && i + 1 < args.length) {
      imgs.push(args[++i]);
    } else if (arg === "--remote") {
      remote = true;
    } else if (arg === "--list") {
      list = true;
    } else if (arg === "--scan") {
      scan = true;
    }
  }

  return { repo, message, remote, list, scan, imgs };
}

/** Resolve --img arguments to absolute paths in the image store */
function resolveImages(imgs: string[]): string[] {
  const resolved: string[] = [];
  for (const query of imgs) {
    const found = findImageByName(query);
    if (!found) {
      console.error(`Error: no image matching "${query}" in ~/.screenshot-agent/`);
      console.error("Run --list to see available images.");
      process.exit(1);
    }
    resolved.push(found.path);
  }
  return resolved;
}

async function runLocal(
  screenshotPaths: string[],
  repo: string,
  message?: string
): Promise<void> {
  const prompt = buildPrompt({ screenshotPaths, repo, message });
  const exitCode = await spawnAgent({ prompt });
  for (const p of screenshotPaths) markProcessed(p);
  process.exit(exitCode);
}

async function runRemote(
  screenshotPaths: string[],
  repo: string,
  message?: string
): Promise<void> {
  const config = loadConfig();

  // Send each image as a separate task (daemon processes via agent-queue)
  const taskIds: string[] = [];
  for (const screenshotPath of screenshotPaths) {
    const taskId = createTaskId();
    const tmpBase = join(tmpdir(), "screenshot-agent-tasks");
    mkdirSync(tmpBase, { recursive: true });

    const payload: TaskPayload = {
      repo,
      message,
      timestamp: Date.now(),
      status: "pending",
    };

    const taskDir = writeTask(tmpBase, taskId, payload, screenshotPath);
    console.log(`Task ${taskId} created`);
    console.log(`Sending to ${config.sshHost}...`);

    await sendTask(config, taskDir, taskId);
    taskIds.push(taskId);
    markProcessed(screenshotPath);
  }

  console.log(`${taskIds.length} task(s) sent. Waiting for results...`);

  // Poll for all results
  let failed = false;
  for (const taskId of taskIds) {
    const result = await pollForResult(config, taskId);
    console.log(`\n--- Task ${taskId} ---`);

    if (result.status === "success") {
      console.log("Fix applied successfully!");
      if (result.pr_url) console.log(`PR: ${result.pr_url}`);
      if (result.branch) console.log(`Branch: ${result.branch}`);
    } else {
      console.error("Agent reported an error:");
      console.error(result.summary.slice(-500));
      failed = true;
    }
  }

  // Pull once at the end if repo is local
  if (existsSync(resolve(repo))) {
    console.log(`\nPulling latest in ${repo}...`);
    const pull = spawn("git", ["pull"], {
      cwd: resolve(repo),
      stdio: "inherit",
    });
    await new Promise<void>((res) => pull.on("close", () => res()));
  }

  if (failed) process.exit(1);
}

async function main() {
  const { repo, message, remote, list, scan, imgs } = parseArgs(process.argv);

  // --scan: ingest images from Desktop/Downloads into ~/.screenshot-agent/
  if (scan) {
    const count = ingestFromScanDirs();
    console.log(`Ingested ${count} image(s) into ~/.screenshot-agent/`);
    if (!repo) process.exit(0);
  }

  // --list: show all images and their status
  if (list) {
    const images = listAllImages();
    if (images.length === 0) {
      console.log("No images in ~/.screenshot-agent/");
      console.log("Run --scan to ingest from ~/Desktop and ~/Downloads.");
    } else {
      const unprocessed = images.filter((i) => !i.isProcessed).length;
      console.log(`${images.length} image(s) in ~/.screenshot-agent/ (${unprocessed} unprocessed):\n`);
      for (const img of images) {
        const status = img.isProcessed ? "done" : "pending";
        console.log(`  [${status}] ${img.name}`);
      }
    }
    process.exit(0);
  }

  if (!repo) {
    console.error("Error: --repo is required\n");
    console.log(USAGE);
    process.exit(1);
  }

  // Resolve images
  let screenshotPaths: string[];

  if (imgs.length > 0) {
    // Specific images requested
    screenshotPaths = resolveImages(imgs);
  } else {
    // Auto-discover latest unprocessed
    const found = findLatestImage();
    if (!found) {
      console.error("No unprocessed images in ~/.screenshot-agent/");
      console.error("Run: screenshot-agent --scan   to ingest from Desktop/Downloads");
      process.exit(1);
    }
    screenshotPaths = [found.path];
  }

  const names = screenshotPaths.map((p) => p.split("/").pop());
  console.log(`Screenshot(s): ${names.join(", ")}`);
  console.log(`Target repo: ${repo}`);
  if (message) console.log(`Context: ${message}`);
  console.log(`Mode: ${remote ? "remote" : "local"}`);
  console.log("---");

  if (remote) {
    await runRemote(screenshotPaths, repo, message);
  } else {
    await runLocal(screenshotPaths, repo, message);
  }
}

main().catch((err) => {
  console.error(err.message || err);
  process.exit(1);
});
