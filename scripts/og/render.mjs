// Renders every social and brand image with headless Chrome from one scene.
// Usage: node scripts/og/render.mjs [name ...]   (default: all; SCALE=2 by default)
// Names: see VARIANTS and LOGOS below. Output paths are relative to the repo root.
// Social images are JPEG (small, accepted everywhere); og-home and og-docs stay PNG so cached URLs keep working.
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync, rmSync, existsSync, readFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const SCALE = Number(process.env.SCALE || 2);
const scaleOf = (v) => v.scale ?? SCALE;
const brand = readFileSync(join(root, "brand.yaml"), "utf8");
const pick = (re) => re.exec(brand)[1].trim();
const name = pick(/^name:\s*(.+)$/m);
const domain = pick(/^docs_url:\s*https?:\/\/(.+)$/m);
const repo = pick(/^repo_url:\s*https?:\/\/(.+)$/m).toLowerCase();

const C = { bg: "#04141a", base: "#06232C", rail1: "#084F67", rail2: "#107292", rail3: "#58B1CE", glow: "#2fb3dc" };
const CHIPS = ["Apache 2.0", "311 templates", "No Kubernetes"];
const FLOW = ["git push", "app.example.com", "build #42 passed", "TLS ready"];

// h1 line 2 is the accent line. scene: labels for [push, live, build, tls].
const COPY = {
  vercel: { cta: "Get started", h1: "Your own Vercel.<br><em>One binary.</em>", sub: "Self-hosted deploys with HTTPS, live logs, metrics and one-click rollback.", chips: CHIPS, scene: FLOW },
  push: { cta: "Get started", h1: "Push to git.<br><em>Get a running app.</em>", sub: "Self-hosted deploys with HTTPS, live logs, metrics and one-click rollback.", chips: CHIPS, scene: FLOW },
  docs: { cta: "Read the docs", h1: "Deploy, observe,<br><em>roll back.</em>", sub: "Install and run the platform on your own servers.", chips: ["Guides", "CLI", "API", "Templates"], scene: FLOW },
  install: { cta: "Install in minutes", h1: "One command.<br><em>Your own cloud.</em>", sub: "Install on a fresh Linux box and open the dashboard in minutes.", chips: ["curl | sh", "Single binary", "Docker"], scene: ["install.sh", "dashboard :8080", "agent online", "TLS ready"] },
  migrate: { cta: "Plan your migration", h1: "Leaving Coolify?<br><em>Bring your apps.</em>", sub: "Read the old server, stage apps and databases, then cut over.", chips: ["Coolify", "Dokploy", "CapRover"], scene: ["docker inspect", "apps staged", "databases copied", "cutover ready"] },
  templates: { cta: "Browse templates", h1: "311 templates.<br><em>One click each.</em>", sub: "Databases, analytics and automation, each a tested compose file.", chips: ["Postgres", "Redis", "n8n"], scene: ["pick", "compose", "deployed", "TLS ready"] },
  canary: { cta: "Read the guide", h1: "Canary deploys,<br><em>no service mesh.</em>", sub: "Send a slice of traffic to the new version, watch, then commit.", chips: ["Weighted traffic", "Instant rollback"], scene: ["deploy v2", "10% traffic", "healthy", "promote"] },
  sleep: { cta: "Read the guide", h1: "Idle apps sleep.<br><em>Requests wake.</em>", sub: "Stop paying in RAM for the side projects nobody visits at 3am.", chips: ["Scale to zero", "Wake on request"], scene: ["no traffic", "sleeping", "request in", "awake"] },
  mcp: { cta: "See the tools", h1: "Ask your server<br><em>what broke.</em>", sub: "An MCP server over the API: logs, metrics, deploys and rollback.", chips: ["MCP", "Read and suggest", "Open API"], scene: ["ask", "logs read", "diagnosis", "rollback ready"] },
};

// OG cards are exactly 1200x630 (scale 1) and keep the message in the middle so square crops stay readable.
const og = (copy, out) => ({ w: 1200, h: 630, out, copy, pad: 64, h1: 70, sub: 24, scene: 800, chips: false, center: true, scale: 1 });
const VARIANTS = {
  "og-home": og("vercel", "docs/assets/og-home.png"),
  "og-docs": og("docs", "docs/assets/og-docs.png"),
  "og-install": og("install", "docs/assets/og/install.jpg"),
  "og-migrate": og("migrate", "docs/assets/og/migrate.jpg"),
  "og-templates": og("templates", "docs/assets/og/templates.jpg"),
  "og-canary": og("canary", "docs/assets/og/canary.jpg"),
  "og-sleep": og("sleep", "docs/assets/og/sleep.jpg"),
  "og-mcp": og("mcp", "docs/assets/og/mcp.jpg"),
  "og-push": og("push", "docs/assets/brand/social/headline-push-1200x630.jpg"),
  github: { w: 1280, h: 640, out: "docs/assets/brand/social/github-social-1280x640.jpg", copy: "vercel", pad: 68, h1: 72, sub: 24, scene: 680, chips: true },
  twitter: { w: 1200, h: 675, out: "docs/assets/brand/social/twitter-card-1200x675.jpg", copy: "vercel", pad: 64, h1: 70, sub: 24, scene: 660, chips: true },
  producthunt: { w: 1270, h: 760, out: "docs/assets/brand/social/producthunt-gallery-1270x760.jpg", copy: "push", pad: 72, h1: 76, sub: 26, scene: 720, chips: true },
  devto: { w: 1000, h: 420, out: "docs/assets/brand/social/devto-cover-1000x420.jpg", copy: "vercel", pad: 52, h1: 48, sub: 0, scene: 440, chips: false },
  square: { w: 1080, h: 1080, out: "docs/assets/brand/social/square-1080x1080.jpg", copy: "vercel", pad: 72, h1: 84, sub: 28, scene: 640, chips: true, stack: true },
  x: { w: 1500, h: 500, out: "docs/assets/brand/social/x-header-1500x500.jpg", copy: "vercel", pad: 72, h1: 56, sub: 0, scene: 560, chips: false, banner: true },
  linkedin: { w: 1584, h: 396, out: "docs/assets/brand/social/linkedin-banner-1584x396.jpg", copy: "vercel", pad: 72, h1: 42, sub: 0, scene: 440, chips: false, banner: true },
};
// Logos render on transparent backgrounds unless bg is set.
const FAV = { w: 64, h: 64, kind: "favicon" };
const LOGOS = {
  "fav-16": { ...FAV, w: 16, h: 16, scale: 1, out: "tmp/fav-16.png" },
  "fav-32": { ...FAV, w: 32, h: 32, scale: 1, out: "docs/public/favicon-32x32.png" },
  "fav-48": { ...FAV, w: 48, h: 48, scale: 1, out: "docs/public/favicon-48x48.png" },
  "fav-64": { ...FAV, w: 64, h: 64, scale: 1, out: "tmp/fav-64.png" },
  "fav-180": { ...FAV, w: 180, h: 180, scale: 1, out: "docs/public/apple-touch-icon.png" },
  "fav-512": { ...FAV, w: 512, h: 512, scale: 1, out: "docs/assets/brand/png/app-icon-512.png" },
  "mark-1024": { w: 1024, h: 1024, out: "docs/assets/brand/png/mark-1024.png", kind: "mark" },
  "mark-on-dark-1024": { w: 1024, h: 1024, out: "docs/assets/brand/png/mark-on-dark-1024.png", kind: "mark", bg: C.bg },
  "wordmark-dark": { w: 1200, h: 300, out: "docs/assets/brand/png/wordmark-for-dark.png", kind: "wordmark", ink: "#eaf6fa" },
  "wordmark-light": { w: 1200, h: 300, out: "docs/assets/brand/png/wordmark-for-light.png", kind: "wordmark", ink: "#06232C" },
};

// One isometric rounded slab: the flat rect is mapped onto the iso plane and
// extruded by stacking darker copies one pixel apart.
function slab(cx, cy, size, fill, thick) {
  const f = cy - 0.5 * size;
  const m = (dy) => `matrix(.866 .5 -.866 .5 ${cx} ${f + dy})`;
  const rx = Math.round(size * 0.16);
  let out = "";
  for (let k = thick; k >= 1; k--) out += `<rect width="${size}" height="${size}" rx="${rx}" fill="${C.base}" transform="${m(k)}"/>`;
  out += `<rect width="${size}" height="${size}" rx="${rx}" fill="${fill}" transform="${m(0)}"/>`;
  out += `<rect width="${size}" height="${size}" rx="${rx}" fill="none" stroke="rgba(255,255,255,.22)" stroke-width="1.2" transform="${m(0)}"/>`;
  return out;
}

function scene(labels) {
  const W = 800;
  const H = 640;
  const cx = 400;
  const layers = [
    { cy: 440, size: 300, fill: C.rail1 },
    { cy: 322, size: 224, fill: C.rail2 },
    { cy: 210, size: 148, fill: C.rail3 },
  ];
  const slabs = layers.map((l) => slab(cx, l.cy, l.size, l.fill, 26)).join("");
  const spine = `<line x1="${cx}" y1="84" x2="${cx}" y2="520" stroke="${C.rail3}" stroke-opacity=".5" stroke-width="2" stroke-dasharray="3 9" stroke-linecap="round"/>`;
  const at = [{ x: 330, y: 30, p: true }, { x: 520, y: 182 }, { x: 556, y: 296 }, { x: 560, y: 408 }];
  const chips = at.map((a, i) => `<div class="sc" style="left:${a.x}px;top:${a.y}px"><i${a.p ? ' class="p"' : ""}></i>${labels[i]}</div>`).join("");
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

const FONT = '-apple-system,"SF Pro Display","Helvetica Neue",Helvetica,Arial,sans-serif';
const MONO = '"JetBrainsMono Nerd Font","JetBrains Mono",Menlo,monospace';

function page(v) {
  const c = COPY[v.copy];
  const scale = v.scene / 800;
  const chips = v.chips ? `<ul class="chips">${c.chips.map((x) => `<li>${x}</li>`).join("")}</ul>` : "";
  const sub = v.sub ? `<p class="sub">${c.sub}</p>` : "";
  const cta = `<div class="cta">${c.cta || "Get started"}<span>${domain}</span><b>&rarr;</b></div>`;
  const link = v.center ? "" : v.banner || v.stack ? `<div class="url">${domain}</div>` : `<div class="url">${domain}</div><div class="tag">${repo}</div>`;
  const copyBox = v.center
    ? `left:50%;top:150px;width:860px;margin-left:-430px;align-items:center;text-align:center;gap:22px`
    : v.stack
    ? `left:${v.pad}px;top:${v.pad + 90}px;width:${v.w - v.pad * 2}px;gap:28px`
    : `left:${v.pad}px;top:${Math.round(v.pad * 0.7) + 40}px;bottom:${Math.round(v.pad * 0.5)}px;width:${Math.round(v.w - v.scene * 0.78 - v.pad)}px;justify-content:center;gap:${Math.round(v.h * 0.045)}px`;
  const sceneBox = v.center
    ? `left:50%;top:50%;margin-left:-400px;margin-top:-40px;transform-origin:center center;opacity:.5;transform:scale(1.1)`
    : v.stack
    ? `left:50%;bottom:${v.pad}px;margin-left:-400px;transform-origin:center bottom`
    : `right:${v.banner ? 20 : 0}px;top:50%;margin-top:-320px;transform-origin:right center`;
  return `<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:${v.w}px;height:${v.h}px;overflow:hidden;background:${C.bg};font-family:${FONT};-webkit-font-smoothing:antialiased}
.stage{position:relative;width:${v.w}px;height:${v.h}px;overflow:hidden;isolation:isolate;
 background:
  radial-gradient(ellipse 55% 90% at 78% 0%,rgba(47,179,220,.34),transparent 72%),
  radial-gradient(ellipse 42% 80% at 4% 0%,rgba(16,114,146,.42),transparent 75%),
  radial-gradient(ellipse 60% 50% at 70% 100%,rgba(88,177,206,.14),transparent 70%),${C.bg}}
.grid{position:absolute;inset:0;z-index:-1;opacity:.5;background-image:linear-gradient(rgba(88,177,206,.07) 1px,transparent 1px),linear-gradient(90deg,rgba(88,177,206,.07) 1px,transparent 1px);background-size:56px 56px;-webkit-mask-image:radial-gradient(ellipse 60% 70% at 70% 50%,#000,transparent 80%)}
.brand{position:absolute;left:${v.pad}px;top:${Math.round(v.pad * 0.7)}px;display:flex;align-items:center;gap:14px;font-size:26px;font-weight:600;letter-spacing:.14em;text-transform:uppercase;color:#eaf6fa}
.copy{position:absolute;${copyBox};display:flex;flex-direction:column}
h1{margin:0;font-weight:700;font-size:${v.h1}px;line-height:1.04;letter-spacing:-.035em;color:#fff}
h1 em{font-style:normal;color:${C.rail3}}
.sub{margin:0;font-size:${v.sub}px;line-height:1.45;color:#c9dae0;max-width:${v.center ? "34em" : "28em"};letter-spacing:-.005em}
.chips{display:flex;flex-wrap:wrap;gap:10px;margin:0;padding:0;list-style:none}
.chips li{padding:7px 14px;border-radius:50px;background:rgba(8,79,103,.28);box-shadow:inset 0 0 0 1px rgba(88,177,206,.28);font-family:${MONO};font-size:15px;color:#d5eaf1}
.url{position:absolute;left:${v.pad}px;bottom:${Math.round(v.pad * 0.55)}px;font-family:${MONO};font-size:22px;color:${C.rail3}}
.tag{position:absolute;right:${v.pad}px;bottom:${Math.round(v.pad * 0.55)}px;font-family:${MONO};font-size:16px;color:#7f98a1;letter-spacing:.04em}
.cta{display:flex;align-items:center;gap:14px;margin-top:6px;padding:14px 22px;border-radius:14px;background:#58B1CE;color:#04141a;font-size:22px;font-weight:700;letter-spacing:-.01em}
.cta span{font-family:${MONO};font-weight:600;font-size:20px;opacity:.78}
.cta b{font-size:24px}
.sc-wrap[data-center] .sc{display:none}
.sc-wrap[data-center]{-webkit-mask-image:linear-gradient(to bottom,transparent 0%,#000 38%)}
.sc-wrap{position:absolute;${sceneBox};width:800px;height:640px;transform:scale(${scale})}
.scene{position:relative}
.sc{position:absolute;display:flex;align-items:center;gap:9px;padding:9px 15px;border-radius:12px;background:rgba(4,20,26,.82);box-shadow:inset 0 0 0 1px rgba(88,177,206,.38),0 10px 30px rgba(0,0,0,.45);font-family:${MONO};font-size:18px;color:#eaf6fa;white-space:nowrap}
.sc i.p{background:#58B1CE;box-shadow:0 0 10px #58B1CE}
.sc i{width:9px;height:9px;border-radius:50%;background:#3ddc97;box-shadow:0 0 10px #3ddc97}
</style></head><body><div class="stage"><div class="grid"></div>
<div class="brand" style="${v.center ? "left:50%;margin-left:-110px;top:44px;" : ""}">${logo(Math.round(v.pad * 0.62))}<span>${name}</span></div>
<div class="copy"><h1>${c.h1}</h1>${sub}${v.center ? cta : chips}</div>
<div class="sc-wrap"${v.center ? ' data-center' : ""}>${scene(c.scene)}</div>
${link}</div></body></html>`;
}

// Tab icon: a dark plate with a brighter ramp so the three rails read at 16px.
export const FAVICON_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><title>${name}</title><rect width="256" height="256" rx="56" fill="#0B0E14"/><rect x="24" y="162" width="208" height="52" rx="16" fill="#06232C"/><rect x="24" y="152" width="208" height="52" rx="16" fill="#107292"/><rect x="54" y="110" width="148" height="52" rx="16" fill="#06232C"/><rect x="54" y="100" width="148" height="52" rx="16" fill="#2FB3DC"/><rect x="84" y="58" width="88" height="52" rx="16" fill="#06232C"/><rect x="84" y="48" width="88" height="52" rx="16" fill="#9FDCEF"/></svg>`;

function logoPage(v) {
  if (v.kind === "favicon") return `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;width:${v.w}px;height:${v.h}px;background:transparent}svg{display:block;width:${v.w}px;height:${v.h}px}</style></head><body>${FAVICON_SVG}</body></html>`;
  const bg = v.bg || "transparent";
  const body = v.kind === "mark"
    ? `<div style="display:flex;width:100%;height:100%;align-items:center;justify-content:center">${logo(Math.round(v.w * (v.bg ? 0.62 : 0.9)))}</div>`
    : `<div style="display:flex;width:100%;height:100%;align-items:center;gap:56px;padding-left:40px;box-sizing:border-box">${logo(Math.round(v.h * 0.7))}<span style="font:600 ${Math.round(v.h * 0.3)}px ${FONT};letter-spacing:.14em;text-transform:uppercase;color:${v.ink}">${name}</span></div>`;
  return `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;width:${v.w}px;height:${v.h}px;background:${bg}}</style></head><body>${body}</body></html>`;
}

const want = process.argv.slice(2);
const all = { ...VARIANTS, ...LOGOS };
const names = want.length ? want : Object.keys(all);
const tmp = join(tmpdir(), `og-render-${process.pid}`);
mkdirSync(tmp, { recursive: true });
if (!existsSync(CHROME)) throw new Error(`Chrome not found at ${CHROME}; set CHROME=/path/to/chrome`);
for (const n of names) {
  const v = all[n];
  if (!v) throw new Error(`unknown name ${n}; known: ${Object.keys(all).join(", ")}`);
  const html = join(tmp, `${n}.html`);
  const out = join(root, v.out);
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(html, v.kind ? logoPage(v) : page(v));
  const jpg = out.endsWith(".jpg");
  const shot = jpg ? join(tmp, `${n}.png`) : out;
  const args = ["--headless=new", "--disable-gpu", "--hide-scrollbars", `--force-device-scale-factor=${scaleOf(v)}`,
    `--window-size=${v.w},${v.h}`, `--screenshot=${shot}`];
  if (v.kind && !v.bg) args.push("--default-background-color=00000000");
  execFileSync(CHROME, [...args, `file://${html}`], { stdio: "pipe" });
  if (jpg) execFileSync("magick", [shot, "-strip", "-sampling-factor", "4:4:4", "-quality", "92", out]);
  console.log(`${n} -> ${v.out} (${v.w * scaleOf(v)}x${v.h * scaleOf(v)})`);
}
if (names.some((n) => n.startsWith("fav-"))) {
  writeFileSync(join(root, "docs/public/favicon-plate.svg"), FAVICON_SVG + "\n");
  const parts = ["tmp/fav-16.png", "docs/public/favicon-32x32.png", "docs/public/favicon-48x48.png", "tmp/fav-64.png"].map((f) => join(root, f));
  if (parts.every(existsSync)) execFileSync("magick", [...parts, join(root, "docs/public/favicon.ico")]);
  rmSync(join(root, "tmp"), { recursive: true, force: true });
}
rmSync(tmp, { recursive: true, force: true });