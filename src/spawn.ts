import { spawn } from "node:child_process";

export interface SpawnOptions {
  prompt: string;
  cwd?: string;
}

export function spawnAgent(opts: SpawnOptions): Promise<number> {
  return new Promise((resolve, reject) => {
    const args = [
      "-p",
      opts.prompt,
      "--dangerously-skip-permissions",
    ];

    const child = spawn("claude", args, {
      cwd: opts.cwd,
      stdio: "inherit",
      env: { ...process.env },
    });

    child.on("error", (err) => {
      if ((err as NodeJS.ErrnoException).code === "ENOENT") {
        reject(new Error("claude CLI not found on PATH. Install it: npm i -g @anthropic-ai/claude-code"));
      } else {
        reject(err);
      }
    });

    child.on("close", (code) => {
      resolve(code ?? 0);
    });
  });
}
