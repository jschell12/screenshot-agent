import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { hostname } from "node:os";
import { AGE_DIR, CONFIG_FILE, QUEUE_REPO_DIR, ensureDirs } from "./config.js";
import { join } from "node:path";

export interface Recipient {
  hostname: string;
  pubkey: string;
}

export interface GitConfig {
  queue_repo: string; // owner/name
  clone_dir: string;
  poll_interval_ms: number;
  branch: string;
  author_name: string;
  author_email: string;
}

export interface AgeConfig {
  identity_file: string;
  pubkey: string;
}

export interface RetentionConfig {
  queue_days: number;
  results_days: number;
}

export interface LookConfig {
  version: 1;
  hostname: string;
  git?: GitConfig;
  age?: AgeConfig;
  recipients?: Recipient[];
  default_recipient?: string;
  retention?: RetentionConfig;
}

/** Sanitize hostname to something filesystem-safe (lowercase alnum + dash) */
export function normalizeHostname(raw: string): string {
  return raw
    .toLowerCase()
    .replace(/\.local$/, "")
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

export function defaultConfig(): LookConfig {
  return {
    version: 1,
    hostname: normalizeHostname(hostname()),
  };
}

export function loadConfig(): LookConfig {
  ensureDirs();
  if (!existsSync(CONFIG_FILE)) {
    const cfg = defaultConfig();
    saveConfig(cfg);
    return cfg;
  }
  const raw = JSON.parse(readFileSync(CONFIG_FILE, "utf-8"));
  // Ensure hostname always present
  if (!raw.hostname) raw.hostname = normalizeHostname(hostname());
  return raw;
}

export function saveConfig(cfg: LookConfig): void {
  ensureDirs();
  writeFileSync(CONFIG_FILE, JSON.stringify(cfg, null, 2) + "\n");
}

export function getRecipient(
  cfg: LookConfig,
  hostnameOrDefault?: string
): Recipient | null {
  if (!cfg.recipients || cfg.recipients.length === 0) return null;
  const target = hostnameOrDefault ?? cfg.default_recipient;
  if (!target) return cfg.recipients[0];
  return cfg.recipients.find((r) => r.hostname === target) ?? null;
}

export function setGitConfig(cfg: LookConfig, queueRepo: string): LookConfig {
  cfg.git = {
    queue_repo: queueRepo,
    clone_dir: QUEUE_REPO_DIR,
    poll_interval_ms: cfg.git?.poll_interval_ms ?? 10_000,
    branch: cfg.git?.branch ?? "main",
    author_name: cfg.git?.author_name ?? "look bot",
    author_email: cfg.git?.author_email ?? "look@localhost",
  };
  return cfg;
}

export function setAgeConfig(
  cfg: LookConfig,
  identityFile: string,
  pubkey: string
): LookConfig {
  cfg.age = { identity_file: identityFile, pubkey };
  return cfg;
}

export function defaultIdentityPath(): string {
  return join(AGE_DIR, "key.txt");
}
