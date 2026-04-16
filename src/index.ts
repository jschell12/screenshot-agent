import { resolve } from "node:path";
import { existsSync } from "node:fs";
import { buildPrompt } from "./prompt.js";
import { spawnAgent } from "./spawn.js";

const USAGE = `Usage: screenshot-agent <screenshot> --repo <repo> [--msg "context"]

  <screenshot>   Path to a screenshot image (.png, .jpg, .webp, .gif)
  --repo <repo>  GitHub repo (owner/name or URL) or local path
  --msg  <msg>   Optional context to guide the agent

Examples:
  screenshot-agent ./bug.png --repo jschell12/my-app
  screenshot-agent ~/Desktop/Screenshot.png --repo jschell12/my-app --msg "fix the button alignment"
  screenshot-agent ./error.png --repo /path/to/local/repo --msg "this error shows on login"`;

function parseArgs(argv: string[]) {
  const args = argv.slice(2);

  if (args.length === 0 || args.includes("--help") || args.includes("-h")) {
    console.log(USAGE);
    process.exit(0);
  }

  let screenshot: string | undefined;
  let repo: string | undefined;
  let message: string | undefined;

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === "--repo" && i + 1 < args.length) {
      repo = args[++i];
    } else if (arg === "--msg" && i + 1 < args.length) {
      message = args[++i];
    } else if (!arg.startsWith("--") && !screenshot) {
      screenshot = arg;
    }
  }

  if (!screenshot) {
    console.error("Error: screenshot path is required\n");
    console.log(USAGE);
    process.exit(1);
  }

  if (!repo) {
    console.error("Error: --repo is required\n");
    console.log(USAGE);
    process.exit(1);
  }

  return { screenshot, repo, message };
}

function validateScreenshot(path: string): string {
  const abs = resolve(path);
  if (!existsSync(abs)) {
    console.error(`Error: screenshot not found: ${abs}`);
    process.exit(1);
  }
  const ext = abs.toLowerCase().split(".").pop();
  if (!ext || !["png", "jpg", "jpeg", "webp", "gif"].includes(ext)) {
    console.error(`Error: unsupported image format: .${ext}`);
    process.exit(1);
  }
  return abs;
}

async function main() {
  const { screenshot, repo, message } = parseArgs(process.argv);
  const screenshotPath = validateScreenshot(screenshot);

  console.log(`Screenshot: ${screenshotPath}`);
  console.log(`Target repo: ${repo}`);
  if (message) console.log(`Context: ${message}`);
  console.log("---");
  console.log("Spawning Claude Code agent...\n");

  const prompt = buildPrompt({ screenshotPath, repo, message });
  const exitCode = await spawnAgent({ prompt });

  process.exit(exitCode);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
