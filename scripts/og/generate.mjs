#!/usr/bin/env node
// Per-page 1200x630 OG cards for the docs, rendered with headless Chrome. No dependencies.
// Usage: node scripts/og/generate.mjs [slug ...] [--all] [--wire]
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, writeFileSync, readFileSync, existsSync, rmSync, statSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { tmpdir } from 'node:os'

const root = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const docs = join(root, 'docs')
// OUT_DIR redirects output so test renders never touch tracked files.
const outDir = process.env.OUT_DIR ? join(process.env.OUT_DIR, 'docs', 'public', 'og') : join(docs, 'public', 'og')
const CHROME = process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

const brandYaml = readFileSync(join(root, 'brand.yaml'), 'utf8')
const val = (k) => brandYaml.match(new RegExp(`^${k}:\\s*(.+)$`, 'm'))?.[1].trim()
const BRAND = { name: val('name'), docsUrl: val('docs_url').replace(/\/$/, ''), logo: JSON.parse(val('logo_svg')) }
const host = BRAND.docsUrl.replace(/^https?:\/\//, '')

// Top pages by traffic intent; --all renders every page in the navigation.
const TOP = [
  'index', 'getting-started', 'installing', 'what-you-need', 'domains-and-ingress', 'environment-and-secrets',
  'troubleshooting', 'coolify-alternative', 'comparison', 'cli-reference', 'api-reference', 'app-spec-reference',
  'multi-node-quickstart', 'architecture', 'deploy-safety', 'tutorials/connect-an-app-to-postgres',
]

const cfg = JSON.parse(readFileSync(join(docs, 'docs.json'), 'utf8'))
const sections = new Map()
const walk = (n, tab = '', group = '') => {
  if (typeof n === 'string') { if (!sections.has(n)) sections.set(n, group || tab); return }
  if (Array.isArray(n)) return n.forEach((x) => walk(x, tab, group))
  if (!n || typeof n !== 'object') return
  const t = n.tab || tab
  const g = n.group || group
  for (const k of ['tabs', 'groups', 'pages']) if (n[k]) walk(n[k], t, g)
}
walk(cfg.navigation)

const unq = (s) => s.trim().replace(/^(["'])([\s\S]*)\1$/, '$2')
const read = (slug) => {
  const f = ['.md', '.mdx'].map((e) => join(docs, slug + e)).find(existsSync)
  if (!f) return null
  const m = readFileSync(f, 'utf8').match(/^---\n([\s\S]*?)\n---/)
  const get = (k) => { const x = m?.[1].match(new RegExp(`^${k}:\\s*(.+)$`, 'm')); return x ? unq(x[1]) : '' }
  return { title: get('title'), description: get('description'), file: f }
}

const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;')
const fontDir = join(root, 'scripts', 'og', 'fonts')
const GRAIN = `url("data:image/svg+xml;utf8,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="300" height="300"><filter id="n" x="0" y="0" width="100%" height="100%"><feTurbulence type="fractalNoise" baseFrequency="0.8" numOctaves="2" stitchTiles="stitch"/><feColorMatrix type="saturate" values="0"/><feComponentTransfer><feFuncR type="linear" slope="3" intercept="-1.15"/><feFuncG type="linear" slope="3" intercept="-1.15"/><feFuncB type="linear" slope="3" intercept="-1.15"/></feComponentTransfer></filter><rect width="100%" height="100%" filter="url(#n)"/></svg>')}")`
// Last word gets the gradient accent.
const accent = (t) => { const i = t.lastIndexOf(' '); return i < 0 ? `<em>${esc(t)}</em>` : `${esc(t.slice(0, i + 1))}<em>${esc(t.slice(i + 1))}</em>` }
const html = ({ label, title, desc }) => `<!doctype html><meta charset="utf-8"><style>
@font-face{font-family:Inter;src:url(file://${fontDir}/inter-var.woff2);font-weight:100 900}
@font-face{font-family:JB;src:url(file://${fontDir}/jetbrains-mono-var.woff2);font-weight:100 800}
*{margin:0;box-sizing:border-box}
body{width:1200px;height:630px;overflow:hidden;background:#050709;color:#fff;font-family:Inter,-apple-system,"Helvetica Neue",Arial,sans-serif;position:relative}
.wash{position:absolute;inset:0;background:radial-gradient(ellipse 60% 70% at 85% 0%,rgba(47,179,220,.16),transparent 70%)}
.glow{position:absolute;inset:0;background:radial-gradient(ellipse 52% 80% at 100% 0%,rgba(47,179,220,.95),transparent 70%),radial-gradient(ellipse 40% 70% at 62% 0%,rgba(70,100,235,.85),transparent 70%),radial-gradient(ellipse 34% 60% at 0% 0%,rgba(20,170,150,.8),transparent 72%),radial-gradient(ellipse 30% 55% at 84% 2%,rgba(238,243,95,.95),transparent 72%);-webkit-mask-image:${GRAIN};mask-image:${GRAIN};-webkit-mask-size:300px 300px;mask-size:300px 300px}
.floor{position:absolute;inset:0;background:linear-gradient(to top,rgba(30,60,70,.30),transparent 38%)}
.frame{position:absolute;left:790px;top:150px;width:620px;height:440px;transform-origin:0 0;transform:perspective(1240px) rotateY(-22deg) rotateX(9deg) rotateZ(-1.5deg);border-radius:16px;overflow:hidden;background:#0a0d10;box-shadow:0 0 0 1px rgba(92,196,232,.55),0 0 54px rgba(47,179,220,.30),-24px 40px 70px rgba(0,0,0,.75)}
.frame img{width:620px;display:block;opacity:.96}
.grain{position:absolute;inset:0;z-index:5;pointer-events:none;background-image:${GRAIN};background-size:300px 300px;opacity:.05;mix-blend-mode:screen}
.c{position:absolute;inset:0;padding:64px 72px;display:flex;flex-direction:column}
.top{display:flex;align-items:center;gap:16px}
.top svg{width:52px;height:52px}
.mono{font-family:JB,"JetBrains Mono",Menlo,monospace}
.label{font-size:22px;letter-spacing:.1em;text-transform:uppercase;color:#cfd6dc}
.main{margin-top:auto;margin-bottom:34px;position:relative;z-index:10}
h1{font-weight:300;font-size:${title.length > 30 ? 52 : 66}px;line-height:1.06;letter-spacing:-.034em;max-width:640px}
h1 em{font-style:normal;background:linear-gradient(90deg,#5cc4e8,#9fe0c8 45%,#eef35f);-webkit-background-clip:text;background-clip:text;color:transparent;padding-right:.04em}
p{margin-top:22px;font-size:24px;line-height:1.4;color:#98a3ab;max-width:600px}
.foot{display:flex;gap:28px;width:640px;border-top:1px solid #232a30;padding-top:20px;font-size:22px;color:#6b757c}
.foot b{color:#5cc4e8;font-weight:400}
</style><div class="wash"></div><div class="glow"></div><div class="floor"></div><div class="frame"><img src="file://${join(root, 'docs/assets/screenshots/dashboard-home.png')}"></div><div class="c">
<div class="top">${BRAND.logo}<span class="label mono">${esc(label)}</span></div>
<div class="main"><h1>${accent(title)}</h1>${desc ? `<p>${esc(desc)}</p>` : ''}</div>
<div class="foot mono"><span>${esc(host)}</span><b>${esc(BRAND.name)}</b></div></div><div class="grain"></div>`

const clip = (s, n) => (s.length <= n ? s : s.slice(0, s.lastIndexOf(' ', n - 1)).replace(/[,.;:]$/, '') + '...')
const tmp = mkdtempSync(join(tmpdir(), 'og-'))
const render = (name, card) => {
  mkdirSync(outDir, { recursive: true })
  const h = join(tmp, name.replace(/\W/g, '_') + '.html')
  const out = join(outDir, name + '.png')
  mkdirSync(dirname(out), { recursive: true })
  writeFileSync(h, html(card))
  const argv = ['--headless=new', '--disable-gpu', '--hide-scrollbars', '--force-device-scale-factor=1', '--window-size=1200,630', `--user-data-dir=${join(tmp, 'profile')}`, `--screenshot=${out}`, `file://${h}`]
  for (let i = 0; ; i++) {
    try { execFileSync(CHROME, argv, { stdio: 'ignore', timeout: 30000 }); break } catch (e) { if (i === 2) throw e }
  }
  if (process.env.OG_QUANTIZE !== '0') { try { execFileSync('magick', [out, '-colors', '256', '-define', 'png:compression-level=9', out], { stdio: 'ignore' }) } catch { /* optional */ } }
  console.log(`${String(Math.round(statSync(out).size / 1024)).padStart(4)} KB  ${out.replace(root + '/', '')}`)
  return out
}

const args = process.argv.slice(2)
const wire = args.includes('--wire')
let slugs = args.filter((a) => !a.startsWith('--'))
if (args.includes('--all')) slugs = [...sections.keys()]
if (!slugs.length) slugs = TOP

render('default', { label: 'Documentation', title: 'Deploy on your own servers', desc: 'Push to git, get a running app with TLS, logs, metrics and rollback.' })
for (const slug of slugs) {
  const p = read(slug)
  if (!p || !p.title) { console.warn('skip', slug); continue }
  const name = slug.replace(/\//g, '-')
  render(name, { label: sections.get(slug) || 'Documentation', title: p.title, desc: clip(p.description, 140) })
  if (wire) {
    let t = readFileSync(p.file, 'utf8')
    if (!/^"og:image":/m.test(t)) {
      const url = `${BRAND.docsUrl}/public/og/${name}.png`
      t = t.replace(/^---\n([\s\S]*?)\n---/, (_, fm) => `---\n${fm}\n"og:image": "${url}"\n"twitter:image": "${url}"\n---`)
      writeFileSync(p.file, t)
    }
  }
}
rmSync(tmp, { recursive: true, force: true })
