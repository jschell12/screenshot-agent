import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  statSync,
  writeFileSync,
  appendFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { basename, extname, join } from "node:path";

const IMAGE_EXTS = new Set([".png", ".jpg", ".jpeg", ".webp", ".gif"]);
const PROCESSED_DIR = join(homedir(), ".screenshot-agent-processed");
const PROCESSED_LOG = join(PROCESSED_DIR, "processed.log");

/** Special watch directories inside ~/Desktop and ~/Downloads */
const WATCH_DIR_NAME = ".screenshot-agent";

/** All directories to scan for images, in priority order */
function scanDirs(): string[] {
  const home = homedir();
  return [
    // Special watch directories first (higher priority)
    join(home, "Desktop", WATCH_DIR_NAME),
    join(home, "Downloads", WATCH_DIR_NAME),
    // Then Desktop and Downloads themselves
    join(home, "Desktop"),
    join(home, "Downloads"),
  ];
}

function isImage(filename: string): boolean {
  return IMAGE_EXTS.has(extname(filename).toLowerCase());
}

/** Ensure the processed tracking directory exists */
function ensureProcessedDir(): void {
  mkdirSync(PROCESSED_DIR, { recursive: true });
  if (!existsSync(PROCESSED_LOG)) {
    writeFileSync(
      PROCESSED_LOG,
      "# Processed images — one absolute path per line\n"
    );
  }
}

/** Get the set of already-processed image paths */
function loadProcessed(): Set<string> {
  ensureProcessedDir();
  const lines = readFileSync(PROCESSED_LOG, "utf-8").split("\n");
  const set = new Set<string>();
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed && !trimmed.startsWith("#")) set.add(trimmed);
  }
  return set;
}

/** Mark an image as processed */
export function markProcessed(absPath: string): void {
  ensureProcessedDir();
  appendFileSync(PROCESSED_LOG, absPath + "\n");
}

/** List all images in a directory, sorted by mtime descending (newest first) */
function listImages(dir: string): { path: string; mtime: number }[] {
  if (!existsSync(dir)) return [];
  const entries = readdirSync(dir, { withFileTypes: true });
  const images: { path: string; mtime: number }[] = [];

  for (const entry of entries) {
    if (!entry.isFile()) continue;
    if (!isImage(entry.name)) continue;
    const fullPath = join(dir, entry.name);
    const stat = statSync(fullPath);
    images.push({ path: fullPath, mtime: stat.mtimeMs });
  }

  return images.sort((a, b) => b.mtime - a.mtime);
}

export interface DiscoveredImage {
  path: string;
  source: "watch-dir" | "desktop" | "downloads";
  isNew: boolean;
}

/**
 * Find the latest unprocessed image.
 *
 * Priority:
 *   1. ~/Desktop/.screenshot-agent/ (watch dir)
 *   2. ~/Downloads/.screenshot-agent/ (watch dir)
 *   3. ~/Desktop (latest unprocessed)
 *   4. ~/Downloads (latest unprocessed)
 *
 * Returns null if no unprocessed images found.
 */
export function findLatestImage(): DiscoveredImage | null {
  const processed = loadProcessed();
  const home = homedir();

  const sources: { dir: string; source: DiscoveredImage["source"] }[] = [
    { dir: join(home, "Desktop", WATCH_DIR_NAME), source: "watch-dir" },
    { dir: join(home, "Downloads", WATCH_DIR_NAME), source: "watch-dir" },
    { dir: join(home, "Desktop"), source: "desktop" },
    { dir: join(home, "Downloads"), source: "downloads" },
  ];

  for (const { dir, source } of sources) {
    const images = listImages(dir);
    for (const img of images) {
      if (!processed.has(img.path)) {
        return { path: img.path, source, isNew: true };
      }
    }
  }

  return null;
}

/**
 * Find a specific image from a directory path.
 * If the path is a directory, return the latest unprocessed image in it.
 * If the path is a file, return it directly.
 */
export function resolveImageArg(arg: string): string | null {
  if (!existsSync(arg)) return null;

  const stat = statSync(arg);
  if (stat.isFile()) return arg;

  if (stat.isDirectory()) {
    const processed = loadProcessed();
    const images = listImages(arg);
    for (const img of images) {
      if (!processed.has(img.path)) return img.path;
    }
    return null;
  }

  return null;
}

/**
 * List all unprocessed images across all scan directories.
 */
export function listUnprocessed(): DiscoveredImage[] {
  const processed = loadProcessed();
  const home = homedir();
  const result: DiscoveredImage[] = [];

  const sources: { dir: string; source: DiscoveredImage["source"] }[] = [
    { dir: join(home, "Desktop", WATCH_DIR_NAME), source: "watch-dir" },
    { dir: join(home, "Downloads", WATCH_DIR_NAME), source: "watch-dir" },
    { dir: join(home, "Desktop"), source: "desktop" },
    { dir: join(home, "Downloads"), source: "downloads" },
  ];

  for (const { dir, source } of sources) {
    for (const img of listImages(dir)) {
      if (!processed.has(img.path)) {
        result.push({ path: img.path, source, isNew: true });
      }
    }
  }

  return result;
}

/** Ensure the special watch directories exist */
export function ensureWatchDirs(): void {
  const home = homedir();
  mkdirSync(join(home, "Desktop", WATCH_DIR_NAME), { recursive: true });
  mkdirSync(join(home, "Downloads", WATCH_DIR_NAME), { recursive: true });
}
