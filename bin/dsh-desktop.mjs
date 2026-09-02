#!/usr/bin/env node
// bin/dsh-desktop.mjs — npm entry point for @deepseek-ai/dsh-desktop.
//
// Ensures the prebuilt dsh-desktop.exe is available locally and spawns it.
// The native shell manages the dsh web backend itself, so the launcher only
// needs to locate/obtain the binary and hand off stdout/stderr.
//
// Binary resolution order:
//   1. %LOCALAPPDATA%\dsh-desktop\dsh-desktop.exe   (postinstall / updater target)
//   2. <this package>\bin\dsh-desktop.exe           (only if published prebuilt)
//   3. download release asset from GitHub Releases  (self-healing fallback)

import { spawn } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { copyFile, readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { fetchReleaseExe } from "../scripts/fetch-exe.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));

// Version is read from this package's own package.json (used to pick the right
// GitHub release asset on the self-healing fallback path).
async function packageVersion() {
  try {
    const pkg = JSON.parse(await readFile(join(__dirname, "..", "package.json"), "utf8"));
    return pkg.version ?? "0.1.0";
  } catch {
    return "0.1.0";
  }
}

function exePath() {
  const local = process.env.LOCALAPPDATA;
  if (local) return join(local, "dsh-desktop", "dsh-desktop.exe");
  return join(__dirname, "dsh-desktop.exe");
}

async function ensureExe() {
  const target = exePath();

  if (existsSync(target)) return target;

  const bundled = join(__dirname, "dsh-desktop.exe");
  if (existsSync(bundled)) {
    mkdirSync(dirname(target), { recursive: true });
    await copyFile(bundled, target);
    return target;
  }

  // Fallback: fetch the release asset for the current package version.
  const version = await packageVersion();
  try {
    await fetchReleaseExe({ version, dest: target });
    return target;
  } catch (err) {
    console.error(`[dsh-desktop] failed to obtain dsh-desktop.exe: ${err.message}`);
    console.error(`[dsh-desktop] install manually from GitHub Releases: https://github.com/andykwokatgithub/dsh-desktop/releases`);
    process.exit(1);
  }
}

const exe = await ensureExe();

const child = spawn(exe, process.argv.slice(2), {
  cwd: process.cwd(),
  stdio: "inherit",
  windowsHide: false,
});

child.on("error", (err) => {
  console.error(`[dsh-desktop] failed to launch ${exe}: ${err.message}`);
  process.exit(1);
});

child.on("exit", (code, signal) => {
  process.exit(signal ? 1 : (code ?? 0));
});
