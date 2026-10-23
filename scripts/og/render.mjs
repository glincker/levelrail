// Renders every social and brand image with headless Chrome: grain-masked glows, light Inter headline, dashboard in a tilted glass frame.
// Usage: node scripts/og/render.mjs [name ...]   (default: all; SCALE=2 by default)
// Names: see VARIANTS and LOGOS below. Output paths are relative to the repo root.
// Social images are JPEG (small, accepted everywhere); og-home and og-docs stay PNG so cached URLs keep working.
import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync, rmSync, existsSync, readFileSync } from "node:fs";
import { join, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
// OUT_DIR redirects every output (same relative paths) so test renders never touch tracked files.
const outRoot = process.env.OUT_DIR ? resolve(process.env.OUT_DIR) : root;
const CHROME = process.env.CHROME || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const SCALE = Number(process.env.SCALE || 2);
const scaleOf = (v) => v.scale ?? SCALE;
const brand = readFileSync(join(root, "brand.yaml"), "utf8");
const pick = (re) => re.exec(brand)[1].trim();
const name = pick(/^name:\s*(.+)$/m);
const domain = pick(/^docs_url:\s*https?:\/\/(.+)$/m);

const C = { bg: "#050709", base: "#06232C", rail1: "#084F67", rail2: "#107292", rail3: "#58B1CE", glow: "#2fb3dc", accent: "#5cc4e8" };
const H1W = Number(process.env.H1W || 700);
const fontDir = join(root, "scripts", "og", "fonts");
const FONTFACE = `@font-face{font-family:Inter;src:url(file://${fontDir}/inter-var.woff2);font-weight:100 900}@font-face{font-family:JB;src:url(file://${fontDir}/jetbrains-mono-var.woff2);font-weight:100 800}`;
const GRAIN = `url("data:image/svg+xml;utf8,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="300" height="300"><filter id="n" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency="0.8" numOctaves="2" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/><feComponentTransfer><feFuncR type="linear" slope="3" intercept="-1.15"/><feFuncG type="linear" slope="3" intercept="-1.15"/><feFuncB type="linear" slope="3" intercept="-1.15"/></feComponentTransfer></filter><rect width="100%" height="100%" filter="url(#n)"/></svg>')}")`;
const CHIPS = ["Apache 2.0", "300+ templates", "No Kubernetes"];

// h1 line 2 is the accent line.
const COPY = {
  vercelstyle: { h1: "Vercel-style deploys,<br><em>on your own server.</em>", sub: "Git push to a live app with HTTPS, logs, metrics and one-click rollback, on hardware you own. No Kubernetes.", chips: CHIPS },
  anotheroption: { h1: "Another option<br><em>for your own server.</em>", sub: "Git push to a live app with HTTPS, logs, metrics and one-click rollback. Open source, one Go binary, no Kubernetes.", chips: CHIPS },
  alternative: { h1: "The self-hosted alternative<br><em>to Vercel and Coolify.</em>", sub: "Deploy with git push, and bring your apps over from Coolify, Dokploy or CapRover with a dry run first.", chips: ["Apache 2.0", "Dry-run import", "300+ templates"] },
  vercel: { cta: "Get started", h1: "Your own Vercel.<br><em>On your server.</em>", sub: "The lightweight, self-hosted alternative to Vercel, Heroku and Coolify.", chips: CHIPS },
  push: { cta: "Get started", h1: "Push to git.<br><em>Get a running app.</em>", sub: "Self-hosted deploys with HTTPS, live logs, metrics and one-click rollback.", chips: CHIPS },
  docs: { cta: "Read the docs", h1: "Deploy, observe,<br><em>roll back.</em>", sub: "Install and run the platform on your own servers.", chips: ["Guides", "CLI", "API", "Templates"] },
  install: { cta: "Install in minutes", h1: "One command.<br><em>Your own cloud.</em>", sub: "Install on a fresh Linux box and open the dashboard in minutes.", chips: ["curl | sh", "Single binary", "Docker"] },
  migrate: { cta: "Plan your migration", h1: "Leaving Coolify?<br><em>Bring your apps.</em>", sub: "Read the old server, stage apps and databases, then cut over.", chips: ["Coolify", "Dokploy", "CapRover"] },
  templates: { cta: "Browse templates", h1: "300+ templates.<br><em>One click each.</em>", sub: "Databases, analytics and automation, each a tested compose file.", chips: ["Postgres", "Redis", "n8n"] },
  canary: { cta: "Read the guide", h1: "Canary deploys,<br><em>no service mesh.</em>", sub: "Send a slice of traffic to the new version, watch, then commit.", chips: ["Weighted traffic", "Instant rollback"] },
  sleep: { cta: "Read the guide", h1: "Idle apps sleep.<br><em>Requests wake.</em>", sub: "Stop paying in RAM for the side projects nobody visits at 3am.", chips: ["Scale to zero", "Wake on request"] },
  mcp: { cta: "See the tools", h1: "Ask your server<br><em>what broke.</em>", sub: "An MCP server over the API: logs, metrics, deploys and rollback.", chips: ["MCP", "Read and suggest", "Open API"] },
};

// Every card: left-aligned copy, a real dashboard screenshot in a tilted glass frame bleeding off the right or bottom edge.
// f = frame (left, top, width); t = copy box (left, top, width); pills = chip offsets relative to the frame.
const SHOT = (f) => `docs/assets/screenshots/${f}.png`;
const wide = (w, h, out, o) => ({ w, h, out, copy: "vercel", shot: "dashboard-home", chips: true, sub: 22, pad: 68, ...o });
const og = (copy, out, shot) => wide(1200, 630, out, { copy, shot, pad: 64, h1: 60, tTop: 152, tW: 530, fx: 640, fy: 100, fw: 860, scale: 1, grain: 0.08, pills: true });
// Hero card: dashboard cropped to the stats row and deploy list (banner out), bleeding off the right and bottom edge.
const home = (out, light) => ({ ...og("vercel", out, "dashboard-home"), light, h1: 64, tTop: 128, tW: 495, sub: 22, fx: 575, fy: 170, fw: 660, fhh: 520, iw: 896, ix: -190, iy: -190, pp: [[560, 128], [985, 128]] });
// Reddit and social headline angles: dark and light twins, files under docs/assets/og/angles.
const ANGLE_FIT = { vercelstyle: { h1: 52, sub: 18 }, anotheroption: { h1: 56, sub: 17 }, alternative: { h1: 40, sub: 20 } };
const angle = (name, copy, shot, light) => ({ ...home(`docs/assets/og/angles/${name}${light ? "-light" : ""}.png`, light), copy, shot, ...ANGLE_FIT[copy] });
const VARIANTS = {
  "og-home": home("docs/assets/og-home.png", false),
  "og-home-light": home("docs/assets/og-home-light.png", true),
  "og-docs": og("docs", "docs/assets/og-docs.png", "app-overview"),
  "og-install": og("install", "docs/assets/og/install.jpg", "get-started"),
  "og-migrate": og("migrate", "docs/assets/og/migrate.jpg", "apps-list"),
  "og-templates": og("templates", "docs/assets/og/templates.jpg", "templates-catalog"),
  "og-canary": og("canary", "docs/assets/og/canary.jpg", "load-balancer"),
  "og-sleep": og("sleep", "docs/assets/og/sleep.jpg", "dashboard-sleeping-apps"),
  "og-mcp": og("mcp", "docs/assets/og/mcp.jpg", "plugins"),
  "og-push": og("push", "docs/assets/brand/social/headline-push-1200x630.jpg", "deploy-live-log"),
  // Conversion card: isometric scene plus the install command. Dark for social, light twin for README <picture>.
  ...Object.fromEntries([["vercel-style", "vercelstyle", "dashboard-home"], ["another-option", "anotheroption", "dashboard-home"], ["alternative", "alternative", "apps-list"]].flatMap(([k, copy, shot]) => [
    [`og-angle-${k}`, angle(k, copy, shot, false)],
    [`og-angle-${k}-light`, angle(k, copy, shot, true)],
  ])),
  "og-home-iso": { w: 1200, h: 630, scale: 1, iso: "dark", out: "docs/assets/og/og-home-iso.png" },
  "og-home-iso-light": { w: 1200, h: 630, scale: 1, iso: "light", out: "docs/assets/og/og-home-iso-light.png" },
  github: wide(1280, 640, "docs/assets/brand/social/github-social-1280x640.jpg", { h1: 66, tTop: 150, tW: 560, fx: 640, fy: 96, fw: 900, pills: true }),
  twitter: wide(1200, 675, "docs/assets/brand/social/twitter-card-1200x675.jpg", { pad: 64, h1: 62, tTop: 165, tW: 540, fx: 620, fy: 112, fw: 860, pills: true }),
  producthunt: wide(1270, 760, "docs/assets/brand/social/producthunt-gallery-1270x760.jpg", { copy: "push", shot: "deploy-live-log", pad: 72, h1: 62, sub: 24, tTop: 200, tW: 600, fx: 690, fy: 130, fw: 960, pills: true }),
  devto: wide(1000, 420, "docs/assets/brand/social/devto-cover-1000x420.jpg", { pad: 52, h1: 44, sub: 0, chips: false, tTop: 130, tW: 470, fx: 520, fy: 50, fw: 640 }),
  square: wide(1080, 1080, "docs/assets/brand/social/square-1080x1080.jpg", { pad: 72, h1: 80, sub: 28, tTop: 170, tW: 940, fx: 150, fy: 640, fw: 1100, tilt: 1, urlTop: true, grain: 0.06, pills: true }),
  x: wide(1500, 500, "docs/assets/brand/social/x-header-1500x500.jpg", { pad: 72, h1: 60, sub: 0, chips: false, tTop: 150, tW: 700, fx: 800, fy: 50, fw: 780 }),
  linkedin: wide(1584, 396, "docs/assets/brand/social/linkedin-banner-1584x396.jpg", { pad: 72, h1: 42, sub: 0, chips: false, tTop: 125, tW: 720, fx: 930, fy: 30, fw: 700 }),
};
// Light twins of the social formats sit next to the dark files as *-light.jpg.
for (const k of ["github", "twitter", "producthunt", "devto", "square", "x", "linkedin"]) {
  VARIANTS[`${k}-light`] = { ...VARIANTS[k], light: true, out: VARIANTS[k].out.replace(/\.jpg$/, "-light.jpg") };
}
// Logos render on transparent backgrounds unless bg is set.
const FAV = { w: 64, h: 64, kind: "favicon" };
const PLATE = { w: 64, h: 64, kind: "plate" };
const LOGOS = {
  "fav-16": { ...FAV, w: 16, h: 16, scale: 1, out: "tmp/fav-16.png" },
  "fav-32": { ...FAV, w: 32, h: 32, scale: 1, out: "docs/public/favicon-32x32.png" },
  "fav-48": { ...FAV, w: 48, h: 48, scale: 1, out: "docs/public/favicon-48x48.png" },
  "fav-64": { ...FAV, w: 64, h: 64, scale: 1, out: "tmp/fav-64.png" },
  "fav-96": { ...FAV, w: 96, h: 96, scale: 1, out: "docs/public/favicon-96x96.png" },
  "fav-180": { ...PLATE, w: 180, h: 180, scale: 1, out: "docs/public/apple-touch-icon.png" },
  "fav-192": { ...PLATE, w: 192, h: 192, scale: 1, out: "docs/public/web-app-manifest-192x192.png" },
  "fav-512": { ...PLATE, w: 512, h: 512, scale: 1, out: "docs/public/web-app-manifest-512x512.png" },
  "mark-1024": { w: 1024, h: 1024, out: "docs/assets/brand/png/mark-1024.png", kind: "mark" },
  "mark-on-dark-1024": { w: 1024, h: 1024, out: "docs/assets/brand/png/mark-on-dark-1024.png", kind: "mark", bg: C.bg },
  "wordmark-dark": { w: 1200, h: 300, out: "docs/assets/brand/png/wordmark-for-dark.png", kind: "wordmark", ink: "#eaf6fa" },
  "wordmark-light": { w: 1200, h: 300, out: "docs/assets/brand/png/wordmark-for-light.png", kind: "wordmark", ink: "#06232C" },
};



function logo(sz, dark = false) {
  const [a, b, c] = dark ? ["#0f7c9e", "#1fa3cb", "#6fd0ee"] : [C.rail1, C.rail2, C.rail3];
  return `<svg width="${sz}" height="${sz}" viewBox="0 0 256 256"><rect x="10" y="174" width="236" height="68" rx="22" fill="${C.base}"/><rect x="10" y="162" width="236" height="68" rx="22" fill="${a}"/><rect x="42" y="103" width="172" height="68" rx="22" fill="${C.base}"/><rect x="42" y="91" width="172" height="68" rx="22" fill="${b}"/><rect x="75" y="32" width="106" height="68" rx="22" fill="${C.base}"/><rect x="75" y="20" width="106" height="68" rx="22" fill="${c}"/></svg>`;
}

const FONT = 'Inter,-apple-system,"Helvetica Neue",Arial,sans-serif';
const MONO = 'JB,"JetBrains Mono",Menlo,monospace';

const LIVE = "#eef35f";
const SITE = domain.replace(/^docs\./, "");

// Shared dark and light palettes: glows are masked by the grain tile so they dissolve into speckle.
function theme(dark) {
  return dark
    ? { bg: "#050709", ink: "#f4f6f8", sub: "#9aa6ae", pillBg: "#090c0f", pillLine: "#2a323a", pillInk: "#cfd6dc", url: "#5cc4e8", tag: "#7d8a93", grad: `linear-gradient(90deg,#5cc4e8 0%,#9fe0c8 45%,${LIVE} 100%)`,
        glows: ["radial-gradient(ellipse 52% 80% at 100% 0%,rgba(47,179,220,.95),transparent 70%)", "radial-gradient(ellipse 40% 70% at 62% 0%,rgba(70,100,235,.85),transparent 70%)", `radial-gradient(ellipse 34% 60% at 0% 0%,rgba(20,170,150,.8),transparent 72%)`, `radial-gradient(ellipse 30% 55% at 84% 2%,rgba(238,243,95,.95),transparent 72%)`],
        soft: "radial-gradient(ellipse 60% 70% at 85% 0%,rgba(47,179,220,.16),transparent 70%)",
        floor: "linear-gradient(to top,rgba(30,60,70,.30),transparent 38%)", blend: "screen", scene: "", logoDark: true }
    : { bg: "#f7f8fb", ink: "#0e1013", sub: "#4a525a", pillBg: "#0b0e11", pillLine: "#2a323a", pillInk: "#e3e8ec", url: "#0d6a86", tag: "#5a646c", grad: "linear-gradient(90deg,#0d6a86 0%,#3f7d6a 55%,#8a8f00 100%)",
        glows: ["radial-gradient(ellipse 52% 80% at 100% 0%,rgba(47,179,220,.9),transparent 70%)", "radial-gradient(ellipse 40% 70% at 62% 0%,rgba(150,130,255,.75),transparent 70%)", "radial-gradient(ellipse 34% 60% at 0% 0%,rgba(90,210,180,.8),transparent 72%)", `radial-gradient(ellipse 30% 55% at 100% 70%,rgba(238,243,95,.85),transparent 72%)`],
        soft: "radial-gradient(ellipse 60% 70% at 85% 0%,rgba(47,179,220,.10),transparent 70%)",
        floor: "linear-gradient(to top,rgba(200,215,225,.45),transparent 30%)", blend: "multiply", logoDark: false };
}

function page(v) {
  const c = COPY[v.copy];
  const dark = !v.light;
  const T = theme(dark);
  const small = v.h < 450;
  const aspect = { "get-started": 1000, "dashboard-sleeping-apps": 1100 }[v.shot] ?? 900;
  const chips = v.chips ? `<ul class="chips">${c.chips.map((x) => `<li>${x}</li>`).join("")}</ul>` : "";
  const sub = v.sub ? `<p class="sub">${c.sub}</p>` : "";
  const fh = v.fhh ?? Math.round((v.fw * aspect) / 1280);
  const tilt = v.tilt ? "perspective(2200px) rotateX(14deg) rotateY(-10deg) rotateZ(-1.5deg)" : `perspective(${Math.round(v.fw * 2.2)}px) rotateY(-12deg) rotateX(3deg) rotateZ(-1deg)`;
  const pill = (x, y, label, ready) => `<div class="pill" style="left:${x}px;top:${y}px"><i${ready ? ' class="g"' : ""}></i>${label}</div>`;
  const pp = v.pp ?? (v.tilt
    ? [[v.fx + 40, v.fy - 40], [v.fx + v.fw * 0.55, v.fy + 10]]
    : [[v.fx - 12, v.fy - 42], [v.fx + v.fw * 0.74, v.fy - 42]]);
  const pills = v.pills ? pill(pp[0][0], pp[0][1], "git push", false) + pill(pp[1][0], pp[1][1], "TLS ready", true) : "";
  const shot = v.light && existsSync(join(root, SHOT(v.shot + "-light"))) ? v.shot + "-light" : v.shot;
  const mask = `-webkit-mask-image:${GRAIN};mask-image:${GRAIN};-webkit-mask-size:300px 300px;mask-size:300px 300px`;
  const ring = dark ? "rgba(92,196,232,.55)" : "rgba(13,106,134,.35)";
  return `<!doctype html><html><head><meta charset="utf-8"><style>${FONTFACE}
html,body{margin:0;width:${v.w}px;height:${v.h}px;overflow:hidden;background:${T.bg};font-family:${FONT};-webkit-font-smoothing:antialiased;color:${T.ink}}
.stage{position:relative;width:${v.w}px;height:${v.h}px;overflow:hidden;isolation:isolate;background:${T.bg}}
.soft{position:absolute;inset:0;background:${T.soft}}
.glow{position:absolute;inset:0;background:${T.glows.join(",")};${mask}}
.floor{position:absolute;inset:0;background:${T.floor}}
.grain{position:absolute;inset:0;z-index:20;pointer-events:none;background-image:${GRAIN};background-size:300px 300px;opacity:${dark ? 0.05 : 0.07};mix-blend-mode:${T.blend}}
.brand{position:absolute;left:${v.pad}px;top:${Math.round(v.pad * 0.72)}px;display:flex;align-items:center;gap:12px;font-size:${small ? 18 : 20}px;font-weight:600;letter-spacing:.14em;text-transform:uppercase;color:${T.ink};z-index:10}
.copy{position:absolute;left:${v.pad}px;top:${v.tTop}px;width:${v.tW}px;z-index:10}
h1{margin:0;font-weight:300;font-size:${v.h1}px;line-height:1.06;letter-spacing:-.034em;color:${T.ink}}
h1 em{font-style:normal;background:${T.grad};-webkit-background-clip:text;background-clip:text;color:transparent;padding-right:.04em}
.sub{margin:${Math.round(v.h1 * 0.36)}px 0 0;font-size:${v.sub}px;line-height:1.4;color:${T.sub};font-weight:400}
.chips{display:flex;flex-wrap:wrap;gap:10px;margin:${Math.round(v.h1 * 0.4)}px 0 0;padding:0;list-style:none}
.chips li{padding:6px 13px;border-radius:50px;background:${dark ? "#090c0f" : "rgba(255,255,255,.7)"};box-shadow:inset 0 0 0 1px ${dark ? "#2a323a" : "#c9d2d9"};font-family:${MONO};font-size:${v.h1 < 62 ? 14 : 15}px;color:${dark ? "#cfd6dc" : "#2c353c"}}
.url{position:absolute;left:${v.pad}px;${v.urlTop ? `top:${Math.round(v.pad * 0.75) + 6}px;left:auto;right:${v.pad}px` : `bottom:${Math.round(v.pad * 0.55)}px`};font-family:${MONO};font-size:${small ? 18 : 20}px;color:${T.url};z-index:10}
.frame{position:absolute;left:${v.fx}px;top:${v.fy}px;width:${v.fw}px;height:${fh}px;transform-origin:0 0;transform:${tilt};border-radius:16px;overflow:hidden;background:${dark ? "#0a0d10" : "#fff"};box-shadow:0 0 0 1px ${ring},0 0 54px ${dark ? "rgba(47,179,220,.30)" : "rgba(47,179,220,.25)"},-28px 44px 80px rgba(0,0,0,${dark ? ".75" : ".28"});z-index:2}
.frame img{position:absolute;width:${v.iw ?? v.fw}px;left:${v.ix ?? 0}px;top:${v.iy ?? 0}px;display:block}
.frame b{position:absolute;inset:0;background:linear-gradient(115deg,rgba(255,255,255,${dark ? ".07" : ".25"}),transparent 35%)}
.pill{position:absolute;display:flex;align-items:center;gap:9px;padding:${small ? 6 : 8}px 15px;border-radius:12px;background:#090c0f;box-shadow:inset 0 0 0 1px #2a323a,0 12px 30px rgba(0,0,0,.5);font-family:${MONO};font-size:${small ? 14 : 17}px;color:#cfd6dc;white-space:nowrap;z-index:12}
.pill i{width:9px;height:9px;border-radius:50%;background:${LIVE};box-shadow:0 0 10px ${LIVE}}
.pill i.g{background:#3ddc97;box-shadow:0 0 10px #3ddc97}
</style></head><body><div class="stage"><div class="soft"></div><div class="glow"></div><div class="floor"></div>
<div class="brand">${logo(Math.round(v.pad * 0.5), dark)}<span>${name}</span></div>
<div class="copy"><h1>${c.h1}</h1>${sub}${chips}</div>
<div class="frame"><img src="file://${join(root, SHOT(shot))}"><b></b></div>${pills}
<div class="url">${domain}</div><div class="grain"></div></div></body></html>`;
}

const ISO_SVG = readFileSync(join(root, "scripts", "og", "assets", "iso-hero.svg"), "utf8");

// Glows are masked by the grain tile (its alpha is the noise), so they dissolve into speckle instead of banding.
function isoPage(v) {
  const dash = false;
  const dark = v.iso === "dark";
  const T = theme(dark);
  const svgVars = dark ? "" : "--ink:#3a4650;--lk:#5b49d6;--teal:#007a68;--live:#e9ef4a";
  const mask = `-webkit-mask-image:${GRAIN};mask-image:${GRAIN};-webkit-mask-size:300px 300px;mask-size:300px 300px`;
  return `<!doctype html><html><head><meta charset="utf-8"><style>${FONTFACE}
html,body{margin:0;width:${v.w}px;height:${v.h}px;overflow:hidden;background:${T.bg};font-family:${FONT};-webkit-font-smoothing:antialiased;color:${T.ink}}
.stage{position:relative;width:${v.w}px;height:${v.h}px;overflow:hidden;isolation:isolate;background:${T.bg}}
.soft{position:absolute;inset:0;background:${T.soft}}
.glow{position:absolute;inset:0;background:${T.glows.join(",")};${mask}}
.floor{position:absolute;inset:0;background:${T.floor}}
.scene{position:absolute;left:540px;top:58px;width:660px;z-index:2}
.scene svg{display:block;width:100%;height:auto;overflow:visible;${svgVars}}
.frame{position:absolute;left:600px;top:112px;width:760px;height:560px;overflow:hidden;border-radius:16px;transform-origin:0 0;transform:perspective(1700px) rotateY(-12deg) rotateX(3deg) rotateZ(-1deg);background:${dark ? "#0a0d10" : "#fff"};box-shadow:0 0 0 1px ${dark ? "rgba(92,196,232,.55)" : "rgba(13,106,134,.35)"},0 0 54px ${dark ? "rgba(47,179,220,.30)" : "rgba(47,179,220,.25)"},-28px 44px 80px rgba(0,0,0,${dark ? ".75" : ".28"});z-index:2}
.frame img{position:absolute;width:1024px;left:-210px;top:-218px;display:block}
.frame b{position:absolute;inset:0;background:linear-gradient(115deg,rgba(255,255,255,${dark ? ".07" : ".25"}),transparent 35%)}
.pill{position:absolute;display:flex;align-items:center;gap:9px;padding:8px 15px;border-radius:12px;background:#090c0f;box-shadow:inset 0 0 0 1px #2a323a,0 12px 30px rgba(0,0,0,.5);font-family:${MONO};font-size:17px;color:#cfd6dc;white-space:nowrap;z-index:12}
.pill i{width:9px;height:9px;border-radius:50%;background:${LIVE};box-shadow:0 0 10px ${LIVE}}
.pill i.g{background:#3ddc97;box-shadow:0 0 10px #3ddc97}
.brand{position:absolute;left:64px;top:46px;display:flex;align-items:center;gap:12px;font-size:20px;font-weight:600;letter-spacing:.14em;text-transform:uppercase;color:${T.ink};z-index:10}
.copy{position:absolute;left:64px;top:128px;width:520px;z-index:10}
h1{margin:0;font-weight:300;font-size:64px;line-height:1.06;letter-spacing:-.034em;color:${T.ink}}
h1 em{font-style:normal;background:${T.grad};-webkit-background-clip:text;background-clip:text;color:transparent;padding-right:.04em}
.sub{margin:22px 0 0;font-size:22px;line-height:1.4;color:${T.sub};font-weight:400;width:${dash ? 500 : 470}px}
.cta{display:flex;align-items:center;gap:10px;margin-top:30px}
.cmd{display:inline-flex;align-items:center;gap:10px;padding:13px 20px;border-radius:12px;background:${T.pillBg};box-shadow:inset 0 0 0 1px ${T.pillLine},0 12px 30px rgba(0,0,0,${dark ? ".5" : ".18"});font-family:${MONO};font-size:17px;color:${T.pillInk};white-space:nowrap}
.cmd b{color:${LIVE};font-weight:700}
.chips{display:flex;gap:10px;margin:22px 0 0;padding:0;list-style:none}
.chips li{padding:6px 13px;border-radius:50px;background:${dark ? "#090c0f" : "rgba(255,255,255,.7)"};box-shadow:inset 0 0 0 1px ${dark ? "#2a323a" : "#c9d2d9"};font-family:${MONO};font-size:14px;color:${dark ? "#cfd6dc" : "#2c353c"}}
.url{position:absolute;left:64px;bottom:34px;font-family:${MONO};font-size:20px;color:${T.url};z-index:10}
.tag{position:absolute;right:64px;bottom:36px;font-family:${MONO};font-size:16px;letter-spacing:.16em;text-transform:uppercase;color:${T.tag};z-index:10}
.grain{position:absolute;inset:0;z-index:20;pointer-events:none;background-image:${GRAIN};background-size:300px 300px;opacity:${dark ? 0.05 : 0.07};mix-blend-mode:${T.blend}}
</style></head><body><div class="stage"><div class="soft"></div><div class="glow"></div><div class="floor"></div>
<div class="brand">${logo(34, T.logoDark)}<span>${name}</span></div>
<div class="copy"><h1>${dash ? COPY.vercel.h1 : "Your own cloud.<br><em>No Kubernetes.</em>"}</h1>
<p class="sub">${dash ? COPY.vercel.sub : "Deploy from a git push. One Go binary, agents that dial out, no SSH."}</p>
${dash ? "" : `<div class="cta"><span class="cmd"><b>$</b>curl -fsSL ${SITE}/install.sh | sh</span></div>`}
<ul class="chips">${(dash ? CHIPS : ["Apache 2.0", "300+ templates"]).map((x) => `<li>${x}</li>`).join("")}</ul></div>
${dash ? `<div class="frame"><img src="file://${join(root, SHOT(v.shot))}"><b></b></div><div class="pill" style="left:590px;top:70px"><i></i>git push</div><div class="pill" style="left:610px;top:548px"><i class="g"></i>TLS ready</div>` : `<div class="scene">${ISO_SVG}</div>`}
<div class="url">${dash ? domain : SITE}</div>${dash ? "" : '<div class="tag">Go / Docker / Caddy</div>'}<div class="grain"></div></div></body></html>`;
}

// Tab icon: a dark plate with a brighter ramp so the three rails read at 16px.
// The tab icon is the transparent mark, identical to the control plane's favicon.
export const FAVICON_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><title>${name}</title><rect x="10" y="174" width="236" height="68" rx="22" fill="#06232C"/><rect x="10" y="162" width="236" height="68" rx="22" fill="#084F67"/><rect x="42" y="103" width="172" height="68" rx="22" fill="#06232C"/><rect x="42" y="91" width="172" height="68" rx="22" fill="#107292"/><rect x="75" y="32" width="106" height="68" rx="22" fill="#06232C"/><rect x="75" y="20" width="106" height="68" rx="22" fill="#58B1CE"/></svg>`;
// iOS and manifest icons need an opaque square, so those use the plate.
export const PLATE_SVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256"><title>${name}</title><rect width="256" height="256" rx="56" fill="#0B0E14"/><rect x="24" y="162" width="208" height="52" rx="16" fill="#06232C"/><rect x="24" y="152" width="208" height="52" rx="16" fill="#107292"/><rect x="54" y="110" width="148" height="52" rx="16" fill="#06232C"/><rect x="54" y="100" width="148" height="52" rx="16" fill="#2FB3DC"/><rect x="84" y="58" width="88" height="52" rx="16" fill="#06232C"/><rect x="84" y="48" width="88" height="52" rx="16" fill="#9FDCEF"/></svg>`;

function logoPage(v) {
  if (v.kind === "plate") return `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;width:${v.w}px;height:${v.h}px;background:transparent}svg{display:block;width:${v.w}px;height:${v.h}px}</style></head><body>${PLATE_SVG}</body></html>`;
  if (v.kind === "favicon") return `<!doctype html><html><head><meta charset="utf-8"><style>html,body{margin:0;width:${v.w}px;height:${v.h}px;background:transparent}svg{display:block;width:${v.w}px;height:${v.h}px}</style></head><body>${FAVICON_SVG}</body></html>`;
  const bg = v.bg || "transparent";
  const body = v.kind === "mark"
    ? `<div style="display:flex;width:100%;height:100%;align-items:center;justify-content:center">${logo(Math.round(v.w * (v.bg ? 0.62 : 0.9)), !!v.bg)}</div>`
    : `<div style="display:flex;width:100%;height:100%;align-items:center;gap:56px;padding-left:40px;box-sizing:border-box">${logo(Math.round(v.h * 0.7), v.ink !== "#06232C")}<span style="font:600 ${Math.round(v.h * 0.3)}px ${FONT};letter-spacing:.14em;text-transform:uppercase;color:${v.ink}">${name}</span></div>`;
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
  const out = join(outRoot, v.out);
  mkdirSync(dirname(out), { recursive: true });
  writeFileSync(html, v.kind ? logoPage(v) : v.iso ? isoPage(v) : page(v));
  const jpg = out.endsWith(".jpg");
  const shot = jpg ? join(tmp, `${n}.png`) : out;
  const args = ["--headless=new", "--disable-gpu", "--hide-scrollbars", `--force-device-scale-factor=${scaleOf(v)}`,
    `--window-size=${v.w},${v.h}`, `--screenshot=${shot}`];
  if (v.kind && !v.bg) args.push("--default-background-color=00000000");
  execFileSync(CHROME, [...args, `file://${html}`], { stdio: "pipe" });
  if (jpg) execFileSync("magick", [shot, "-strip", "-sampling-factor", "4:4:4", "-quality", "70", out]);
  else if (!v.kind) execFileSync("magick", [shot, "-colors", "256", "-define", "png:compression-level=9", out]);
  console.log(`${n} -> ${v.out} (${v.w * scaleOf(v)}x${v.h * scaleOf(v)})`);
}
if (names.some((n) => n.startsWith("fav-"))) {
  writeFileSync(join(outRoot, "docs/public/favicon.svg"), FAVICON_SVG + "\n");
  writeFileSync(join(outRoot, "docs/public/favicon-plate.svg"), PLATE_SVG + "\n");
  const parts = ["tmp/fav-16.png", "docs/public/favicon-32x32.png", "docs/public/favicon-48x48.png"].map((f) => join(outRoot, f));
  if (parts.every(existsSync)) execFileSync("magick", [...parts, join(outRoot, "docs/public/favicon.ico")]);
  rmSync(join(outRoot, "tmp"), { recursive: true, force: true });
}
rmSync(tmp, { recursive: true, force: true });