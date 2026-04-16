import { spawn } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, relative } from "node:path";

function git(
  args: string[],
  cwd?: string,
  input?: Buffer
): Promise<{ stdout: string; stderr: string; code: number }> {
  return new Promise((resolve, reject) => {
    const child = spawn("git", args, {
      cwd,
      stdio: input ? ["pipe", "pipe", "pipe"] : ["ignore", "pipe", "pipe"],
    });

    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];

    child.stdout!.on("data", (c) => stdout.push(c));
    child.stderr!.on("data", (c) => stderr.push(c));

    child.on("error", reject);
    child.on("close", (code) =>
      resolve({
        stdout: Buffer.concat(stdout).toString("utf-8"),
        stderr: Buffer.concat(stderr).toString("utf-8"),
        code: code ?? 1,
      })
    );

    if (input) {
      child.stdin!.write(input);
      child.stdin!.end();
    }
  });
}

async function gitOrThrow(args: string[], cwd?: string): Promise<string> {
  const { stdout, stderr, code } = await git(args, cwd);
  if (code !== 0) {
    throw new Error(`git ${args.join(" ")} failed (exit ${code}): ${stderr}`);
  }
  return stdout;
}

export function repoUrl(slug: string): string {
  if (slug.startsWith("http") || slug.startsWith("git@")) return slug;
  return `git@github.com:${slug}.git`;
}

/** Clone the repo if it doesn't exist, otherwise fetch + reset to origin/branch */
export async function ensureCloned(
  slug: string,
  cloneDir: string,
  branch: string
): Promise<void> {
  if (!existsSync(join(cloneDir, ".git"))) {
    mkdirSync(dirname(cloneDir), { recursive: true });
    await gitOrThrow(["clone", repoUrl(slug), cloneDir]);
    await gitOrThrow(["checkout", branch], cloneDir).catch(async () => {
      // Branch may not exist yet — create it
      await gitOrThrow(["checkout", "-b", branch], cloneDir);
    });
  } else {
    await gitOrThrow(["fetch", "origin"], cloneDir);
    await gitOrThrow(["checkout", branch], cloneDir).catch(async () => {
      await gitOrThrow(["checkout", "-b", branch], cloneDir);
    });
  }
}

/** Pull with rebase; autostash protects uncommitted changes */
export async function pullRebase(
  cloneDir: string,
  branch: string
): Promise<void> {
  const { code, stderr } = await git(
    ["pull", "--rebase=true", "--autostash", "origin", branch],
    cloneDir
  );
  if (code !== 0) {
    // If origin/branch doesn't exist yet (brand-new repo), that's fine
    if (/couldn't find remote ref|no such ref/i.test(stderr)) return;
    throw new Error(`git pull failed: ${stderr}`);
  }
}

/** Stage specific paths, commit with a message, push. Retries once on non-FF. */
export async function commitAndPush(
  cloneDir: string,
  paths: string[],
  message: string,
  branch: string,
  authorName?: string,
  authorEmail?: string
): Promise<void> {
  if (paths.length === 0) return;

  // Stage by explicit path (covers add + rm)
  for (const p of paths) {
    await gitOrThrow(["add", "--all", "--", p], cloneDir);
  }

  // Bail if nothing actually changed
  const { stdout: statusOut } = await git(
    ["status", "--porcelain"],
    cloneDir
  );
  if (!statusOut.trim()) return;

  const commitArgs = ["commit", "-m", message];
  if (authorName && authorEmail) {
    commitArgs.unshift(
      "-c",
      `user.name=${authorName}`,
      "-c",
      `user.email=${authorEmail}`
    );
  }
  await gitOrThrow(commitArgs, cloneDir);

  // Try push, retry once on non-fast-forward
  const first = await git(["push", "origin", branch], cloneDir);
  if (first.code === 0) return;

  if (/non-fast-forward|rejected|fetch first/i.test(first.stderr)) {
    await pullRebase(cloneDir, branch);
    await gitOrThrow(["push", "origin", branch], cloneDir);
    return;
  }

  throw new Error(`git push failed: ${first.stderr}`);
}

export function listFiles(cloneDir: string, subdir: string): string[] {
  const dir = join(cloneDir, subdir);
  if (!existsSync(dir)) return [];
  return readdirSync(dir)
    .filter((name) => !name.startsWith("."))
    .map((name) => join(subdir, name));
}

export function readRepoFile(cloneDir: string, relPath: string): Buffer {
  return readFileSync(join(cloneDir, relPath));
}

export function writeRepoFile(
  cloneDir: string,
  relPath: string,
  data: Buffer
): void {
  const full = join(cloneDir, relPath);
  mkdirSync(dirname(full), { recursive: true });
  writeFileSync(full, data);
}

export function repoFileExists(cloneDir: string, relPath: string): boolean {
  return existsSync(join(cloneDir, relPath));
}

export async function gitRm(
  cloneDir: string,
  relPath: string
): Promise<void> {
  await gitOrThrow(["rm", "--", relPath], cloneDir);
}
