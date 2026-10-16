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
const outDir = join(docs, 'public', 'og')
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
const html = ({ label, title, desc }) => `<!doctype html><meta charset="utf-8"><style>
*{margin:0;box-sizing:border-box}
body{width:1200px;height:630px;overflow:hidden;background:#000;color:#fff;font-family:Geist,Inter,-apple-system,"Helvetica Neue",Arial,sans-serif;position:relative}
.wash{position:absolute;inset:0;background:radial-gradient(54% 92% at 50% 0%,rgba(182,171,255,.5),transparent 72%),radial-gradient(38% 78% at 8% 0%,rgba(0,203,170,.42),transparent 72%),radial-gradient(36% 70% at 94% 0%,rgba(238,243,95,.34),transparent 72%)}
.fade{position:absolute;inset:0;background:linear-gradient(180deg,rgba(0,0,0,0) 15%,#000 78%)}
.c{position:absolute;inset:0;padding:64px 72px;display:flex;flex-direction:column}
.top{display:flex;align-items:center;gap:16px}
.top svg{width:52px;height:52px}
.mono{font-family:"Geist Mono","SF Mono",ui-monospace,Menlo,monospace}
.label{font-size:22px;letter-spacing:.1em;text-transform:uppercase;color:#cfcfd3}
.main{margin-top:auto;margin-bottom:34px}
h1{font-weight:300;font-size:${title.length > 44 ? 64 : 80}px;line-height:1.05;letter-spacing:-.025em;max-width:1000px}
p{margin-top:22px;font-size:28px;line-height:1.35;color:#a9a9b2;max-width:960px}
.foot{display:flex;justify-content:space-between;border-top:1px solid rgba(255,255,255,.12);padding-top:20px;font-size:22px;color:#8d8d96}
.foot b{color:#eef35f;font-weight:400}
</style><div class="wash"></div><div class="fade"></div><div class="c">
<div class="top">${BRAND.logo}<span class="label mono">${esc(label)}</span></div>
<div class="main"><h1>${esc(title)}</h1>${desc ? `<p>${esc(desc)}</p>` : ''}</div>
<div class="foot mono"><span>${esc(host)}</span><b>${esc(BRAND.name)}</b></div></div>`

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
  if (process.env.OG_QUANTIZE !== '0') { try { execFileSync('magick', [out, '-colors', '128', '-define', 'png:compression-level=9', out], { stdio: 'ignore' }) } catch { /* optional */ } }
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
