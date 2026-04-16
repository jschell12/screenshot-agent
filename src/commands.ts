import { chmodSync, existsSync, readFileSync } from "node:fs";
import {
  extractPubkeyFromIdentity,
  generateKeypair,
} from "./age.js";
import {
  commitAndPush,
  ensureCloned,
  pullRebase,
  repoFileExists,
  readRepoFile,
  writeRepoFile,
} from "./gitTransport.js";
import {
  defaultIdentityPath,
  loadConfig,
  saveConfig,
  setAgeConfig,
  setGitConfig,
  type LookConfig,
  type Recipient,
} from "./gitConfig.js";
import { QUEUE_REPO_DIR, ensureDirs } from "./config.js";
import { join } from "node:path";

/** `look init-keys` — generate keypair, publish pubkey if queue repo configured */
export async function cmdInitKeys(): Promise<number> {
  ensureDirs();
  const cfg = loadConfig();
  const identityPath = cfg.age?.identity_file ?? defaultIdentityPath();

  if (existsSync(identityPath)) {
    const pubkey = extractPubkeyFromIdentity(identityPath);
    console.log(`Identity already exists at ${identityPath}`);
    console.log(`Public key: ${pubkey}`);
    setAgeConfig(cfg, identityPath, pubkey);
    saveConfig(cfg);
    await publishOwnPubkey(cfg);
    return 0;
  }

  console.log(`Generating age keypair at ${identityPath}...`);
  const pubkey = await generateKeypair(identityPath);
  try {
    chmodSync(identityPath, 0o600);
  } catch {}

  setAgeConfig(cfg, identityPath, pubkey);
  saveConfig(cfg);

  console.log(`\nPublic key: ${pubkey}`);
  console.log(`\nShare this pubkey with other machines so they can encrypt to you:`);
  console.log(`  look add-recipient ${cfg.hostname} --pubkey ${pubkey}`);

  await publishOwnPubkey(cfg);
  return 0;
}

/** Publish pubkey to queue repo's pubkeys/<hostname>.pub (idempotent) */
async function publishOwnPubkey(cfg: LookConfig): Promise<void> {
  if (!cfg.git || !cfg.age) return;

  try {
    await ensureCloned(cfg.git.queue_repo, cfg.git.clone_dir, cfg.git.branch);
    await pullRebase(cfg.git.clone_dir, cfg.git.branch);
  } catch (err) {
    console.warn(
      `Note: could not reach queue repo to publish pubkey: ${err instanceof Error ? err.message : err}`
    );
    return;
  }

  const relPath = `pubkeys/${cfg.hostname}.pub`;
  const existing = repoFileExists(cfg.git.clone_dir, relPath)
    ? readRepoFile(cfg.git.clone_dir, relPath).toString("utf-8").trim()
    : "";

  if (existing === cfg.age.pubkey) {
    console.log(`Pubkey already published at ${relPath}`);
    return;
  }

  writeRepoFile(
    cfg.git.clone_dir,
    relPath,
    Buffer.from(cfg.age.pubkey + "\n")
  );
  await commitAndPush(
    cfg.git.clone_dir,
    [relPath],
    `Publish pubkey for ${cfg.hostname}`,
    cfg.git.branch,
    cfg.git.author_name,
    cfg.git.author_email
  );
  console.log(`Published ${relPath} to ${cfg.git.queue_repo}`);
}

/** `look queue-init <owner/repo>` — register and clone the queue repo */
export async function cmdQueueInit(slug: string): Promise<number> {
  if (!slug || !/^[\w-]+\/[\w.-]+$/.test(slug)) {
    console.error(`Error: invalid repo slug "${slug}" (expected owner/name)`);
    return 1;
  }

  const cfg = loadConfig();
  setGitConfig(cfg, slug);
  saveConfig(cfg);

  console.log(`Cloning ${slug} into ${cfg.git!.clone_dir}...`);
  await ensureCloned(slug, cfg.git!.clone_dir, cfg.git!.branch);

  // Scaffold directories with .gitkeep
  const dirs = ["queue", "results", "pubkeys", "processed"];
  const touched: string[] = [];
  for (const d of dirs) {
    const keep = `${d}/.gitkeep`;
    if (!repoFileExists(cfg.git!.clone_dir, keep)) {
      writeRepoFile(cfg.git!.clone_dir, keep, Buffer.from(""));
      touched.push(keep);
    }
  }

  if (touched.length > 0) {
    await commitAndPush(
      cfg.git!.clone_dir,
      touched,
      "Scaffold queue repo directories",
      cfg.git!.branch,
      cfg.git!.author_name,
      cfg.git!.author_email
    );
  }

  console.log(`Queue repo ready: ${slug}`);

  if (cfg.age) {
    await publishOwnPubkey(cfg);
  } else {
    console.log(`\nNext: run 'look init-keys' to generate your age keypair.`);
  }
  return 0;
}

/** `look add-recipient <hostname> [--pubkey <pubkey>] [--default]` */
export async function cmdAddRecipient(
  hostname: string,
  opts: { pubkey?: string; asDefault?: boolean }
): Promise<number> {
  if (!hostname) {
    console.error("Error: hostname required");
    return 1;
  }

  const cfg = loadConfig();
  let pubkey = opts.pubkey;

  if (!pubkey) {
    // Try to fetch from queue repo
    if (!cfg.git) {
      console.error(
        "Error: no pubkey given and no queue repo configured. Run 'look queue-init <repo>' first, or pass --pubkey."
      );
      return 1;
    }
    try {
      await ensureCloned(cfg.git.queue_repo, cfg.git.clone_dir, cfg.git.branch);
      await pullRebase(cfg.git.clone_dir, cfg.git.branch);
    } catch (err) {
      console.error(
        `Error: could not reach queue repo: ${err instanceof Error ? err.message : err}`
      );
      return 1;
    }
    const relPath = `pubkeys/${hostname}.pub`;
    if (!repoFileExists(cfg.git.clone_dir, relPath)) {
      console.error(`Error: no pubkey found at ${relPath} in ${cfg.git.queue_repo}`);
      console.error("Ask that machine's owner to run 'look init-keys' first.");
      return 1;
    }
    pubkey = readRepoFile(cfg.git.clone_dir, relPath)
      .toString("utf-8")
      .trim();
  }

  if (!pubkey.startsWith("age1")) {
    console.error(`Error: "${pubkey}" doesn't look like an age pubkey`);
    return 1;
  }

  cfg.recipients = cfg.recipients ?? [];
  const existing = cfg.recipients.find((r) => r.hostname === hostname);
  if (existing) {
    existing.pubkey = pubkey;
    console.log(`Updated recipient ${hostname}`);
  } else {
    cfg.recipients.push({ hostname, pubkey });
    console.log(`Added recipient ${hostname}`);
  }

  if (opts.asDefault || !cfg.default_recipient) {
    cfg.default_recipient = hostname;
    console.log(`Default recipient: ${hostname}`);
  }

  saveConfig(cfg);
  return 0;
}

/** `look list-recipients` */
export async function cmdListRecipients(): Promise<number> {
  const cfg = loadConfig();
  console.log(`Hostname: ${cfg.hostname}`);
  if (cfg.age) console.log(`Self pubkey: ${cfg.age.pubkey}`);
  if (cfg.git) console.log(`Queue repo: ${cfg.git.queue_repo}`);
  console.log("");

  if (!cfg.recipients || cfg.recipients.length === 0) {
    console.log("No recipients configured.");
    return 0;
  }

  console.log("Recipients:");
  for (const r of cfg.recipients) {
    const marker = r.hostname === cfg.default_recipient ? " (default)" : "";
    console.log(`  ${r.hostname}${marker}`);
    console.log(`    ${r.pubkey}`);
  }
  return 0;
}
