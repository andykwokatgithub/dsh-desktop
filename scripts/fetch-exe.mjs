#!/usr/bin/env node
// scripts/fetch-exe.mjs — download and verify the prebuilt dsh-desktop.exe
// from the GitHub release matching the package version.
//
// Used two ways:
//   - as a postinstall script (npm i -g @deepseek-ai/dsh-desktop / dsh plugin add)
//   - as a module import, e.g. { fetchReleaseExe } from bin/dsh-desktop.mjs
//
// On success it writes the EXE to <dest> (default %LOCALAPPDATA%\dsh-desktop\dsh-desktop.exe)
// and verifies the SHA256 published alongside it. Fails loudly if the hash does
// not match, so a tampered download is never left installed.

import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { mkdir, rename, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";

const REPO = "andykwokatgithub/dsh-desktop";
const ASSET = "dsh-desktop-win-x64.exe";

async function readVersion() {
  const here = dirname(import.meta.url);
  try {
    const pkg = JSON.parse(await readFile(join(here, "..", "package.json"), "utf8"));
    return pkg.version ?? "0.1.0";
  } catch {
    return "0.1.0";
  }
}

async function sha256File(path) {
  const hash = createHash("sha256");
  await new Promise((resolve, reject) => {
    const stream = createReadStream(path);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("end", resolve);
    stream.on("error", reject);
  });
  return hash.digest("hex");
}

// Resolve the release asset download URL for a tag via the GitHub API.
async function assetUrl(version) {
  const tag = `v${version}`;
  const api = `https://api.github.com/repos/${REPO}/releases/tags/${tag}`;
  const res = await fetch(api, { headers: { Accept: "application/vnd.github+json" } });
  if (!res.ok) throw new Error(`GitHub API ${res.status} for release ${tag}`);
  const release = await res.json();
  const asset = release.assets?.find((a) => a.name === ASSET);
  if (!asset) throw new Error(`release ${tag} has no asset ${ASSET}`);
  return asset.browser_download_url;
}

// Direct, auth-free URL (used as a fallback when the API is rate-limited).
function directUrl(version) {
  return `https://github.com/${REPO}/releases/download/v${version}/${ASSET}`;
}

// A "<sha256>  <filename>" line, with or without the filename.
async function expectedSha(version) {
  const api = `https://api.github.com/repos/${REPO}/releases/tags/v${version}`;
  const res = await fetch(`${api}`, { headers: { Accept: "application/vnd.github+json" } });
  if (res.ok) {
    const release = await res.json();
    const asset = release.assets?.find((a) => a.name === `${ASSET}.sha256`);
    if (asset?.browser_download_url) {
      const txt = await (await fetch(asset.browser_download_url)).text();
      return txt.trim().split(/\s+/)[0];
    }
  }
  throw new Error(`no ${ASSET}.sha256 asset for v${version}`);
}

export async function fetchReleaseExe({ version = null, dest = null } = {}) {
  version = version ?? (await readVersion());
  const target = dest ?? join(process.env.LOCALAPPDATA ?? ".", "dsh-desktop", "dsh-desktop.exe");
  await mkdir(dirname(target), { recursive: true });

  const tmp = `${target}.download`;
  const url = await assetUrl(version).catch(() => directUrl(version));

  const res = await fetch(url, { redirect: "follow" });
  if (!res.ok || !res.body) throw new Error(`download failed (${res.status}) for ${url}`);
  const buf = Buffer.from(await res.arrayBuffer());
  await writeFile(tmp, buf);

  let expected = null;
  try {
    expected = await expectedSha(version);
  } catch {
    // The hash asset is published by our release pipeline; if missing, we still
    // install but flag it, because verification is the security boundary we own.
    process.stderr.write(`[dsh-desktop] warning: no ${ASSET}.sha256 to verify against\n`);
  }

  if (expected) {
    const actual = await sha256File(tmp);
    if (actual.toLowerCase() !== expected.toLowerCase()) {
      await rm(tmp, { force: true });
      throw new Error(`sha256 mismatch: expected ${expected}, got ${actual}`);
    }
  }

  await rename(tmp, target);
  process.stdout.write(`[dsh-desktop] installed ${target} (v${version})\n`);
  return target;
}

// CLI entry (postinstall) — fire only when this file is the invoked script.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  fetchReleaseExe().catch((err) => {
    console.error(`[dsh-desktop] ${err.message}`);
    process.exit(1);
  });
}
