// Build a Windows .ico (and a 256px PNG) from the DeepSeek Harness favicon.svg.
// Renders a DeepSeek-blue rounded-square background with the whale mark in white.
//
// Usage:  node tools/make-icon.mjs
//
// Requires: sharp (resolvable from the dsh node_modules via NODE_PATH).
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..");
const outDir = join(root, "assets");
mkdirSync(outDir, { recursive: true });

// sharp lives in the installed dsh package; resolve it from there because the
// workspace itself has no node_modules.
const dshNodeModules =
  "C:/Users/AndyKwok/AppData/Roaming/npm/node_modules/@deepseek-ai/dsh/node_modules";
const require = createRequire(join(dshNodeModules, "noop.js"));
const sharp = require("sharp");

// Path to the canonical logo in the installed dsh package.
const svgPath =
  "C:/Users/AndyKwok/AppData/Roaming/npm/node_modules/@deepseek-ai/dsh/node_modules/@deepseek-ai/dsh-web-frontend/dist/favicon.svg";
const svg = readFileSync(svgPath, "utf8");

// Extract the whale path `d` from the favicon.
const m = svg.match(/<path[^>]*\bd="([^"]+)"/s);
if (!m) throw new Error("could not find the whale path in favicon.svg");
const d = m[1];

// DeepSeek brand blue + rounded-square app-icon background.
const blue = "#4d6bfe";
const size = 256;
const radius = 56;

// Map the 50x50 favicon viewBox into a centered, scaled sub-region.
const scale = (size * 0.60) / 50;
const tx = (size - 50 * scale) / 2;
const ty = (size - 50 * scale) / 2;

const composite = `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">
  <rect x="0" y="0" width="${size}" height="${size}" rx="${radius}" fill="${blue}"/>
  <g transform="translate(${tx.toFixed(3)} ${ty.toFixed(3)}) scale(${scale.toFixed(4)})">
    <path d="${d}" fill="#ffffff"/>
  </g>
</svg>`;

const sizes = [16, 32, 48, 64, 128, 256];
const pngs = [];
for (const s of sizes) {
  const buf = await sharp(Buffer.from(composite)).resize(s, s, { fit: "fill" }).png().toBuffer();
  pngs.push({ size: s, buf });
}

// Pack PNGs into an ICO container (Vista+ supports PNG-encoded entries).
const count = pngs.length;
const header = Buffer.alloc(6);
header.writeUInt16LE(0, 0); // reserved
header.writeUInt16LE(1, 2); // type: icon
header.writeUInt16LE(count, 4); // image count

let offset = 6 + 16 * count;
const dir = Buffer.alloc(16 * count);
const images = [];
pngs.forEach(({ size, buf }, i) => {
  const e = dir.subarray(i * 16, i * 16 + 16);
  e[0] = size >= 256 ? 0 : size; // width (0 = 256)
  e[1] = size >= 256 ? 0 : size; // height
  e[2] = 0; // color palette
  e[3] = 0; // reserved
  e.writeUInt16LE(1, 4); // planes
  e.writeUInt16LE(32, 6); // bit depth
  e.writeUInt32LE(buf.length, 8); // bytes in resource
  e.writeUInt32LE(offset, 12); // offset
  offset += buf.length;
  images.push(buf);
});

const ico = Buffer.concat([header, dir, ...images]);
const icoPath = join(outDir, "deepseek.ico");
writeFileSync(icoPath, ico);
writeFileSync(join(outDir, "deepseek-256.png"), pngs[pngs.length - 1].buf);

console.log("wrote", icoPath, `(${ico.length} bytes)`);
console.log("wrote", join(outDir, "deepseek-256.png"));
