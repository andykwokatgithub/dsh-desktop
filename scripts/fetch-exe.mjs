#!/usr/bin/env node
// scripts/fetch-exe.mjs — make the dsh-desktop.exe available locally for npm
// users. Since 0.2.2 the prebuilt Windows binary is BUNDLED in the package, so
// the normal path is a pure local copy (no network). If the binary is missing
// (e.g. a git-hosted install that didn't carry it), we fall back to downloading
// the release asset from GitHub with SHA256 verification.
//
// Used two ways:
//   - as a postinstall script (npm i -g @andykwok/dsh-desktop / dsh plugin add)
//   - as a module import, e.g. { fetchReleaseExe } from bin/dsh-desktop.mjs

import { createHash } from "node:crypto";
import { createReadStream } from "node:fs";
import { copyFile, mkdir, rename, rm, stat, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { readFile } from "node:fs/promises";
import { fileURLToPath, pathToFileURL } from "node:url";

const REPO = "andykwokatgithub/dsh-desktop";
const ASSET = "dsh-desktop-win-x64.exe";
const SHA_ASSET = `${ASSET}.sha256`;

// Package root = scripts/ -> ..
const __dirname = dirname(fileURLToPath(import.meta.url));
const pkgRoot = join(__dirname, "..");
const bundled = join(pkgRoot, ASSET);

const mustExist = async (p) => {
  try {
    return (await stat(p)).isFile();
  } catch {
    return false;
  }
};

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

// If the package ships the binary, copy it into place and report success.
async function copyBundled(dest) {
  try {
    if (await mustExist(bundled)) {
      await mkdir(dirname(dest), { recursive: true });
      await copyFile(bundled, dest);
      process.stdout.write(`[dsh-desktop] installed bundled ${dest}\n`);
      return true;
    }
  } catch {
    /* fall through to download */
  }
  return false;
}

async function releaseByTag(tag) {
  const res = await fetch(`https://api.github.com/repos/${REPO}/releases/tags/${tag}`, {
    headers: { Accept: "application/vnd.github+json", "User-Agent": "dsh-desktop-installer" },
  });
  if (!res.ok) throw new Error(`no release ${tag}`);
  return res.json();
}

async function latestRelease() {
  const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, {
    headers: { Accept: "application/vnd.github+json", "User-Agent": "dsh-desktop-installer" },
  });
  if (!res.ok) throw new Error(`no latest release (HTTP ${res.status})`);
  return res.json();
}

function pickAsset(release) {
  const asset = release.assets?.find((a) => a.name === ASSET);
  if (!asset) throw new Error(`release has no ${ASSET} asset`);
  return asset.browser_download_url;
}

async function pickSha(release) {
  const shaAsset = release.assets?.find((a) => a.name === SHA_ASSET);
  if (!shaAsset?.browser_download_url) return "";
  const txt = await (await fetch(shaAsset.browser_download_url)).text();
  return txt.trim().split(/\s+/)[0] ?? "";
}

// Resolve the download URL + expected SHA. Preference: exact package version,
// then the latest stable release.
async function resolveAsset(version) {
  try {
    const rel = await releaseByTag(`v${version}`);
    return { url: pickAsset(rel), sha: await pickSha(rel), tag: `v${version}` };
  } catch {
    const rel = await latestRelease();
    return { url: pickAsset(rel), sha: await pickSha(rel), tag: rel.tag_name ?? "latest" };
  }
}

export async function fetchReleaseExe({ version = null, dest = null } = {}) {
  const target = dest ?? join(process.env.LOCALAPPDATA ?? ".", "dsh-desktop", "dsh-desktop.exe");

  // Preferred: copy the bundled binary (no network).
  if (await copyBundled(target)) return target;

  // Fallback: download from GitHub (with UA, so corporate proxies/CDN accept it)
  // and verify SHA256.
  if (version === null) {
    try {
      const pkg = JSON.parse(await readFile(join(pkgRoot, "package.json"), "utf8"));
      version = pkg.version ?? "0.0.0";
    } catch {
      version = "0.0.0";
    }
  }
  await mkdir(dirname(target), { recursive: true });

  const { url, sha, tag } = await resolveAsset(version);
  const tmp = `${target}.download`;

  const res = await fetch(url, {
    redirect: "follow",
    headers: { "User-Agent": "dsh-desktop-installer" },
  });
  if (!res.ok || !res.body) throw new Error(`download failed (${res.status}) for ${url}`);
  const buf = Buffer.from(await res.arrayBuffer());
  await writeFile(tmp, buf);

  if (sha) {
    const actual = await sha256File(tmp);
    if (actual.toLowerCase() !== sha.toLowerCase()) {
      await rm(tmp, { force: true });
      throw new Error(`sha256 mismatch: expected ${sha}, got ${actual}`);
    }
  } else {
    process.stderr.write(`[dsh-desktop] warning: no ${SHA_ASSET} to verify against (${tag})\n`);
  }

  await rename(tmp, target);
  process.stdout.write(`[dsh-desktop] installed ${target} (release ${tag})\n`);
  return target;
}

// CLI entry (postinstall) — fire only when this file is the invoked script.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  fetchReleaseExe().catch((err) => {
    console.error(`[dsh-desktop] ${err.message}`);
    process.exit(1);
  });
}
