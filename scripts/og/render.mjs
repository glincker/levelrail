// Renders the social and brand images with headless Chrome from one scene.
// Usage: node scripts/og/render.mjs [og-home og-docs github x linkedin ...]   (default: all)
// Output: docs/assets/*.png (og-*) and docs/assets/brand/social/*.png
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync, rmSync, existsSync, readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const brand = readFileSync(join(root, "brand.yaml"), "utf8");
const name = /^name:\s*(.+)$/m.exec(brand)[1].trim();
const domain = /^docs_url:\s*https?:\/\/(.+)$/m.exec(brand)[1].trim();
const repo = /^repo_url:\s*https?:\/\/(.+)$/m.exec(brand)[1].trim().toLowerCase();

const COPY = {
  home: { h1: 'Push to git.<br><em>Get a running app.</em>', sub: "Self-hosted deploys with HTTPS, live logs, metrics and rollback. One binary, no Kubernetes.", chips: ["Apache 2.0", "311 templates", "Single binary"] },
  docs: { h1: "Deploy, observe,<br><em>roll back.</em>", sub: "Documentation for installing and running Levelrail on your own servers.", chips: ["Guides", "CLI", "API", "Templates"] },
};

// out is relative to the repo root; scene is the scene width in px.
const VARIANTS = {
  "og-home": { w: 1200, h: 630, out: "docs/assets/og-home.png", copy: "home", pad: 64, h1: 68, sub: 23, scene: 640, chips: true },
  "og-docs": { w: 1200, h: 630, out: "docs/assets/og-docs.png", copy: "docs", pad: 64, h1: 68, sub: 23, scene: 640, chips: true },
  github: { w: 1280, h: 640, out: "docs/assets/brand/social/github-social-1280x640.png", copy: "home", pad: 68, h1: 72, sub: 24, scene: 680, chips: true },
  x: { w: 1500, h: 500, out: "docs/assets/brand/social/x-header-1500x500.png", copy: "home", pad: 72, h1: 56, sub: 0, scene: 560, chips: false, banner: true },
  linkedin: { w: 1584, h: 396, out: "docs/assets/brand/social/linkedin-banner-1584x396.png", copy: "home", pad: 72, h1: 42, sub: 0, scene: 440, chips: false, banner: true },
};

const C = { bg: "#04141a", base: "#06232C", rail1: "#084F67", rail2: "#107292", rail3: "#58B1CE", glow: "#2fb3dc" };

// One isometric rounded slab: the flat rect is mapped onto the iso plane and
// extruded by stacking darker copies one pixel apart.
function slab(cx, cy, size, fill, thick) {
  const e = cx - 0.866 * 0;
  const centre = (size / 2) * 0.866 - (size / 2) * 0.866; // x offset is zero for a square
  const f = cy - 0.5 * (size / 2 + size / 2);
  const m = (dy) => `matrix(.866 .5 -.866 .5 ${e + centre} ${f + dy})`;
  const rx = Math.round(size * 0.16);
  let out = "";
  for (let k = thick; k >= 1; k--) out += `<rect width="${size}" height="${size}" rx="${rx}" fill="${C.base}" transform="${m(k)}"/>`;
  out += `<rect width="${size}" height="${size}" rx="${rx}" fill="${fill}" transform="${m(0)}"/>`;
  out += `<rect width="${size}" height="${size}" rx="${rx}" fill="none" stroke="rgba(255,255,255,.22)" stroke-width="1.2" transform="${m(0)}"/>`;
  return out;
}

function scene() {
  const W = 800;
  const H = 640;
  const cx = 400;
  const layers = [
    { cy: 440, size: 300, fill: C.rail1, chip: { t: "TLS ready", x: 560, y: 408 } },
    { cy: 322, size: 224, fill: C.rail2, chip: { t: "build #42 passed", x: 556, y: 296 } },
    { cy: 210, size: 148, fill: C.rail3, chip: { t: "app.example.com", x: 520, y: 182 } },
    { chip: { t: "git push", x: 330, y: 30, push: true } },
  ];
  const slabs = layers.filter((l) => l.size).map((l) => slab(cx, l.cy, l.size, l.fill, 26)).join("");
  const spine = `<line x1="${cx}" y1="84" x2="${cx}" y2="520" stroke="${C.rail3}" stroke-opacity=".5" stroke-width="2" stroke-dasharray="3 9" stroke-linecap="round"/>`;
  const chips = layers.map((l) => `<div class="sc" style="left:${l.chip.x}px;top:${l.chip.y}px"><i${l.chip.push ? ' class="p"' : ""}></i>${l.chip.t}</div>`).join("");
  return `<div class="scene" style="width:${W}px;height:${H}px">
<svg width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" xmlns="http://www.w3.org/2000/svg">
<defs><radialGradient id="pool" cx=".5" cy=".5" r=".5"><stop offset="0" stop-color="${C.glow}" stop-opacity=".45"/><stop offset="1" stop-color="${C.glow}" stop-opacity="0"/></radialGradient></defs>
<ellipse cx="${cx}" cy="500" rx="360" ry="120" fill="url(#pool)"/>
${spine}${slabs}
</svg>${chips}</div>`;
}

function logo(sz) {
  return `<svg width="${sz}" height="${sz}" viewBox="0 0 256 256"><rect x="10" y="174" width="236" height="68" rx="22" fill="${C.base}"/><rect x="10" y="162" width="236" height="68" rx="22" fill="${C.rail1}"/><rect x="42" y="103" width="172" height="68" rx="22" fill="${C.base}"/><rect x="42" y="91" width="172" height="68" rx="22" fill="${C.rail2}"/><rect x="75" y="32" width="106" height="68" rx="22" fill="${C.base}"/><rect x="75" y="20" width="106" height="68" rx="22" fill="${C.rail3}"/></svg>`;
}

const GRAIN = "url(\"data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='220' height='220'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='.9' numOctaves='2' stitchTiles='stitch'/><feColorMatrix values='0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 .55 0'/></filter><rect width='220' height='220' filter='url(%23n)'/></svg>\")";

function page(v) {
  const c = COPY[v.copy];
  const scale = v.scene / 800;
  const chips = v.chips ? `<ul class="chips">${c.chips.map((x) => `<li>${x}</li>`).join("")}</ul>` : "";
  const sub = v.sub ? `<p class="sub">${c.sub}</p>` : "";
  const link = v.banner ? `<div class="url">${domain}</div>` : `<div class="url">${domain}</div><div class="tag">${repo}</div>`;
  return `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:${v.w}px;height:${v.h}px;overflow:hidden;background:${C.bg};
 font-family:-apple-system,"SF Pro Display","Helvetica Neue",Helvetica,Arial,sans-serif;-webkit-font-smoothing:antialiased}
.stage{position:relative;width:${v.w}px;height:${v.h}px;overflow:hidden;isolation:isolate;
 background:
  radial-gradient(ellipse 55% 90% at 78% 0%,rgba(47,179,220,.34),transparent 72%),
  radial-gradient(ellipse 42% 80% at 4% 0%,rgba(16,114,146,.42),transparent 75%),
  radial-gradient(ellipse 60% 50% at 70% 100%,rgba(88,177,206,.14),transparent 70%),${C.bg}}
.grain{position:absolute;inset:0;z-index:5;pointer-events:none;opacity:.16;mix-blend-mode:overlay;background-image:${GRAIN};background-size:220px 220px}
.grid{position:absolute;inset:0;z-index:-1;opacity:.5;background-image:linear-gradient(rgba(88,177,206,.07) 1px,transparent 1px),linear-gradient(90deg,rgba(88,177,206,.07) 1px,transparent 1px);background-size:56px 56px;
 -webkit-mask-image:radial-gradient(ellipse 60% 70% at 70% 50%,#000,transparent 80%)}
.brand{position:absolute;left:${v.pad}px;top:${Math.round(v.pad * 0.7)}px;display:flex;align-items:center;gap:14px;font-size:26px;font-weight:600;letter-spacing:.14em;text-transform:uppercase;color:#eaf6fa}
.copy{position:absolute;left:${v.pad}px;top:${Math.round(v.pad * 0.7) + 40}px;bottom:${Math.round(v.pad * 0.5)}px;width:${Math.round(v.w - v.scene * 0.78 - v.pad)}px;display:flex;flex-direction:column;justify-content:center;gap:${Math.round(v.h * 0.045)}px}
h1{margin:0;font-weight:700;font-size:${v.h1}px;line-height:1.04;letter-spacing:-.035em;color:#fff}
h1 em{font-style:normal;color:${C.rail3}}
.sub{margin:0;font-size:${v.sub}px;line-height:1.45;color:#b9ccd3;max-width:28em;letter-spacing:-.005em}
.chips{display:flex;flex-wrap:wrap;gap:10px;margin:0;padding:0;list-style:none}
.chips li{padding:7px 14px;border-radius:50px;background:rgba(8,79,103,.28);box-shadow:inset 0 0 0 1px rgba(88,177,206,.28);font-family:"JetBrainsMono Nerd Font","JetBrains Mono",Menlo,monospace;font-size:15px;color:#d5eaf1}
.url{position:absolute;left:${v.pad}px;bottom:${Math.round(v.pad * 0.55)}px;font-family:"JetBrainsMono Nerd Font","JetBrains Mono",Menlo,monospace;font-size:22px;color:${C.rail3}}
.tag{position:absolute;right:${v.pad}px;bottom:${Math.round(v.pad * 0.55)}px;font-family:"JetBrainsMono Nerd Font","JetBrains Mono",Menlo,monospace;font-size:16px;color:#7f98a1;letter-spacing:.04em}
.sc-wrap{position:absolute;right:${v.banner ? 20 : 0}px;top:50%;width:800px;height:640px;margin-top:-320px;transform:scale(${scale});transform-origin:right center}
.scene{position:relative}
.sc{position:absolute;display:flex;align-items:center;gap:9px;padding:9px 15px;border-radius:12px;background:rgba(4,20,26,.82);box-shadow:inset 0 0 0 1px rgba(88,177,206,.38),0 10px 30px rgba(0,0,0,.45);font-family:"JetBrainsMono Nerd Font","JetBrains Mono",Menlo,monospace;font-size:18px;color:#eaf6fa;white-space:nowrap}
.sc i.p{background:#58B1CE;box-shadow:0 0 10px #58B1CE}
.sc i{width:9px;height:9px;border-radius:50%;background:#3ddc97;box-shadow:0 0 10px #3ddc97}
</style></head><body><div class="stage"><div class="grid"></div>
<div class="brand">${logo(Math.round(v.pad * 0.62))}<span>${name}</span></div>
<div class="copy"><h1>${c.h1}</h1>${sub}${chips}</div>
<div class="sc-wrap">${scene()}</div>
${link}<div class="grain"></div></div></body></html>`;
}

const want = process.argv.slice(2);
const names = want.length ? want : Object.keys(VARIANTS);
const tmp = join(tmpdir(), `og-render-${process.pid}`);
mkdirSync(tmp, { recursive: true });
if (!existsSync(CHROME)) throw new Error(`Chrome not found at ${CHROME}; set CHROME=/path/to/chrome`);
for (const n of names) {
  const v = VARIANTS[n];
  if (!v) throw new Error(`unknown variant ${n}; known: ${Object.keys(VARIANTS).join(", ")}`);
  const html = join(tmp, `${n}.html`);
  const out = join(root, v.out);
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(html, page(v));
  execFileSync(CHROME, ["--headless=new", "--disable-gpu", "--hide-scrollbars", "--force-device-scale-factor=1",
    `--window-size=${v.w},${v.h}`, `--screenshot=${out}`, `file://${html}`], { stdio: "pipe" });
  console.log(`${n} -> ${v.out}`);
}
rmSync(tmp, { recursive: true, force: true });
