import { spawn } from "node:child_process";
import { readFileSync } from "node:fs";

function ageError(err: NodeJS.ErrnoException, cmd: string): Error {
  if (err.code === "ENOENT") {
    return new Error(
      `${cmd} not found on PATH.\n  Install with: brew install age`
    );
  }
  return err;
}

/** Pipe stdin → process → stdout, return collected stdout buffer */
function run(
  cmd: string,
  args: string[],
  stdin?: Buffer
): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const child = spawn(cmd, args, {
      stdio: ["pipe", "pipe", "pipe"],
    });

    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];

    child.stdout.on("data", (c) => stdout.push(c));
    child.stderr.on("data", (c) => stderr.push(c));

    child.on("error", (err) => reject(ageError(err as NodeJS.ErrnoException, cmd)));
    child.on("close", (code) => {
      if (code !== 0) {
        reject(
          new Error(
            `${cmd} ${args.join(" ")} failed (exit ${code}): ${Buffer.concat(stderr).toString("utf-8")}`
          )
        );
        return;
      }
      resolve(Buffer.concat(stdout));
    });

    if (stdin) {
      child.stdin.write(stdin);
      child.stdin.end();
    } else {
      child.stdin.end();
    }
  });
}

/** Encrypt plaintext to one or more age recipients */
export async function encryptToRecipients(
  plaintext: Buffer,
  recipients: string[]
): Promise<Buffer> {
  if (recipients.length === 0) {
    throw new Error("No recipients specified for encryption");
  }
  const args: string[] = [];
  for (const r of recipients) args.push("-r", r);
  return run("age", args, plaintext);
}

/** Decrypt ciphertext using a private identity file */
export async function decryptWithIdentity(
  ciphertext: Buffer,
  identityFile: string
): Promise<Buffer> {
  return run("age", ["-d", "-i", identityFile], ciphertext);
}

/** Generate a new age keypair. Returns the pubkey. */
export async function generateKeypair(outPath: string): Promise<string> {
  await run("age-keygen", ["-o", outPath]);
  return extractPubkeyFromIdentity(outPath);
}

/** Extract pubkey from the `# public key:` comment line in an identity file */
export function extractPubkeyFromIdentity(identityFile: string): string {
  const content = readFileSync(identityFile, "utf-8");
  const match = content.match(/^#\s*public key:\s*(age1\w+)/m);
  if (!match) throw new Error(`No public key found in ${identityFile}`);
  return match[1];
}
