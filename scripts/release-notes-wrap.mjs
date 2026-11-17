#!/usr/bin/env node
// Wraps a release body with a branded header (logo, title, badges, links, image paths)
// and a footer (contributor avatars, compare link). Prints the final markdown.
// Usage: node scripts/release-notes-wrap.mjs --version 1.4.0 --prev v1.3.2 --body-file f [--contributors a,b]
// Every value below can be overridden by the env var named in the config block.
import fs from 'node:fs'
import { pathToFileURL } from 'node:url'

const env = (k, d) => (process.env[k] !== undefined && process.env[k] !== '' ? process.env[k] : d)

export function config() {
  const repo = env('RELEASE_REPO', env('GITHUB_REPOSITORY', 'glincker/levelrail'))
  const owner = repo.split('/')[0].toLowerCase()
  return {
    repo,
    product: env('RELEASE_PRODUCT', 'Levelrail'),
    docsUrl: env('RELEASE_DOCS_URL', 'https://docs.levelrail.com').replace(/\/$/, ''),
    branch: env('RELEASE_DEFAULT_BRANCH', 'main'),
    logoPath: env('RELEASE_LOGO_PATH', 'docs/assets/brand/png/mark-1024.png'),
    logoDarkPath: env('RELEASE_LOGO_DARK_PATH', 'docs/assets/brand/png/mark-on-dark-1024.png'),
    logoSize: env('RELEASE_LOGO_SIZE', '96'),
    badgeColor: env('RELEASE_BADGE_COLOR', '107292'),
    installCmd: env('RELEASE_INSTALL_CMD', 'curl -fsSL https://levelrail.com/install.sh | sudo sh'),
    registry: env('RELEASE_REGISTRY', `ghcr.io/${owner}`),
    hubNamespace: env('DOCKERHUB_NAMESPACE', 'levelrail'),
    images: env('RELEASE_IMAGES', 'levelrail,levelrail-agent').split(',').filter(Boolean),
    primaryTag: env('IMAGE_PRIMARY_TAG', ''),
    imageTags: env('IMAGE_TAGS', ''),
    mirrored: env('CP_MIRRORED', 'false') === 'true',
    discordUrl: env('RELEASE_DISCORD_URL', 'https://discord.gg/Ar5pcaZB99'),
    discordId: env('RELEASE_DISCORD_ID', '829168897080557579'),
    avatarSize: env('RELEASE_AVATAR_SIZE', '48'),
    botPattern: env('RELEASE_BOT_PATTERN', '\\[bot\\]$|^dependabot|^release-please|^github-actions'),
    theme: env('RELEASE_THEME', ''),
    docsPages: {
      install: env('RELEASE_DOC_INSTALL', 'installing'),
      upgrade: env('RELEASE_DOC_UPGRADE', 'upgrade-safely'),
      features: env('RELEASE_DOC_FEATURES', 'features'),
      verify: env('RELEASE_DOC_VERIFY', 'verify-a-release'),
      changelog: env('RELEASE_DOC_CHANGELOG', 'changelog'),
      faq: env('RELEASE_DOC_FAQ', 'faq'),
    },
  }
}

// Built from char codes so this file itself carries no dash characters.
const LONG_DASHES = new RegExp(`\\s*[${String.fromCharCode(8211)}${String.fromCharCode(8212)}]\\s*`, 'g')
const noDash = (s) => s.replace(LONG_DASHES, ', ')

// Block markers keep re-runs idempotent: old wrapper blocks are dropped first.
export const HEAD_BEGIN = '<!-- release-wrap:start -->'
export const HEAD_END = '<!-- release-wrap:end -->'
export const FOOT_BEGIN = '<!-- release-footer:start -->'
export const FOOT_END = '<!-- release-footer:end -->'

export function stripBlocks(body) {
  const cut = (s, a, b) => {
    const i = s.indexOf(a)
    const j = s.indexOf(b)
    if (i < 0 || j < i) return s
    return s.slice(0, i) + s.slice(j + b.length)
  }
  return cut(cut(body, HEAD_BEGIN, HEAD_END), FOOT_BEGIN, FOOT_END).trim()
}

// The wrapper footer owns contributors and the compare link, so the generator's
// own copies are dropped; its other footer links stay.
export function dedupeBody(body) {
  const lines = body.split('\n')
  const out = []
  let skipping = false
  for (const line of lines) {
    if (/^##\s+contributors\s*$/i.test(line)) {
      skipping = true
      continue
    }
    if (skipping) {
      if (!/^(##\s|<!--|\*\*Full changelog|\[Release page\])/i.test(line)) continue
      skipping = false
    }
    if (/^\**Full changelog\**:?\**\s*https?:\/\/\S+\s*$/i.test(line)) continue
    out.push(line.replace(/^\*\*Full changelog:\*\*\s*\[[^\]]*\]\([^)]*\)(\s*\|\s*)?/i, ''))
  }
  return out.join('\n').replace(/\n{3,}/g, '\n\n').trim()
}

// First feature bullet becomes the one-line theme when none is given.
export function extractTheme(body) {
  let inSection = false
  for (const line of body.split('\n')) {
    if (/^#{2,3}\s/.test(line)) {
      inSection = /^#{2,3}\s+(highlights|features|new)\b/i.test(line)
      continue
    }
    if (!inSection) continue
    const m = /^\s*[*-]\s+(.*)$/.exec(line)
    if (!m) continue
    let t = m[1]
    t = t.replace(/\s+\(\[[^\]]*\]\([^)]*\)\).*$/, '').replace(/\s+by @\S+.*$/, '')
    t = t.replace(/\*\*([^*:]*):?\*\*:?\s*/g, '$1: ').replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/`/g, '')
    t = noDash(t.trim()).replace(/\.$/, '')
    if (t) return t
  }
  return ''
}

export function humans(logins, botPattern) {
  const bot = new RegExp(botPattern, 'i')
  const seen = new Set()
  const out = []
  for (const raw of logins) {
    const l = String(raw || '').trim()
    if (!/^[A-Za-z0-9][A-Za-z0-9-]{0,38}$/.test(l) || bot.test(l) || seen.has(l.toLowerCase())) continue
    seen.add(l.toLowerCase())
    out.push(l)
  }
  return out
}

export function wrap({ version, prev, body, contributors }, cfg = config()) {
  const v = version.replace(/^v/, '')
  const tag = `v${v}`
  const web = `https://github.com/${cfg.repo}`
  const raw = `https://raw.githubusercontent.com/${cfg.repo}/${cfg.branch}`
  const docs = (p) => `${cfg.docsUrl}/${p}`
  const clean = dedupeBody(stripBlocks(body))
  const theme = noDash(cfg.theme) || extractTheme(clean)
  const title = theme ? `${cfg.product} ${v}: ${theme}` : `${cfg.product} ${v}`
  const hub = `${cfg.hubNamespace}/${cfg.images[0]}`
  const badge = (alt, img, href) => `<a href="${href}"><img src="${img}" alt="${alt}"></a>`
  // Shields static badges escape a literal hyphen by doubling it.
  const shieldsVersion = encodeURIComponent(v).split('-').join('--')
  const badges = [
    badge('Release', `https://img.shields.io/badge/release-${shieldsVersion}-${cfg.badgeColor}?style=flat`, `${web}/releases/tag/${tag}`),
    badge('License', `https://img.shields.io/github/license/${cfg.repo}?style=flat`, `${web}/blob/${cfg.branch}/LICENSE`),
    badge('Docker pulls', `https://img.shields.io/docker/pulls/${hub}?style=flat&logo=docker&label=docker%20pulls`, `https://hub.docker.com/r/${hub}`),
    badge('GitHub stars', `https://img.shields.io/github/stars/${cfg.repo}?style=flat&logo=github`, `${web}/stargazers`),
    badge('Discord', `https://img.shields.io/discord/${cfg.discordId}?style=flat&logo=discord&logoColor=white&label=discord&color=5865F2`, cfg.discordUrl),
  ].join('\n  ')

  const compare = prev ? `${web}/compare/${prev}...${tag}` : `${web}/releases/tag/${tag}`
  const links = [
    `<a href="${docs(cfg.docsPages.install)}">Install</a>`,
    `<a href="${docs(cfg.docsPages.upgrade)}">Upgrade guide</a>`,
    `<a href="${docs(cfg.docsPages.changelog)}">Changelog</a>`,
    `<a href="${docs(cfg.docsPages.features)}">Features</a>`,
    `<a href="${docs(cfg.docsPages.verify)}">Verify this release</a>`,
    cfg.docsPages.faq ? `<a href="${docs(cfg.docsPages.faq)}">FAQ</a>` : '',
    `<a href="${compare}">Full diff</a>`,
    `[Discussions](${web}/discussions)`,
  ].filter(Boolean).join(' &middot; ')

  const primary = cfg.primaryTag || tag
  const tags = (cfg.imageTags || primary).split(/\s+/).filter(Boolean)
  const pulls = []
  for (const img of cfg.images) {
    pulls.push(`docker pull ${cfg.registry}/${img}:${primary}`)
    if (cfg.mirrored) pulls.push(`docker pull docker.io/${cfg.hubNamespace}/${img}:${primary}`)
  }
  const getIt = [
    '## Get it',
    '',
    '```bash',
    cfg.installCmd,
    ...pulls,
    '```',
    '',
    `Image tags pushed: ${tags.map((t) => `\`${t}\``).join(' ')}`,
  ].join('\n')

  const head = [
    HEAD_BEGIN,
    '<div align="center">',
    '<picture>',
    `  <source media="(prefers-color-scheme: dark)" srcset="${raw}/${cfg.logoDarkPath}">`,
    `  <img src="${raw}/${cfg.logoPath}" alt="${cfg.product} logo" width="${cfg.logoSize}" height="${cfg.logoSize}">`,
    '</picture>',
    '',
    `<h2>${title}</h2>`,
    `<p>${cfg.product}, a lightweight, self-hosted deployment platform: git push to production on your own servers and cloud accounts.</p>`,
    '<p>',
    `  ${badges}`,
    '</p>',
    `<p>${links}</p>`,
    '</div>',
    '',
    getIt,
    HEAD_END,
  ].join('\n')

  const owner = cfg.repo.split('/')[0]
  const avatars = humans(contributors || [], cfg.botPattern)
    .map((u) => `<a href="https://github.com/${u}"><img src="https://github.com/${u}.png?size=${cfg.avatarSize}" width="${cfg.avatarSize}" height="${cfg.avatarSize}" alt="@${u}"></a>`)
    .join('\n')
  const foot = [
    FOOT_BEGIN,
    '---',
    '',
    '### Contributors to this release',
    '',
    avatars || `<a href="https://github.com/${owner}">@${owner}</a>`,
    '',
    `Full Changelog: ${compare}`,
    '',
    `Thank you to everyone who built, tested and reported on ${cfg.product} ${v}.`,
    FOOT_END,
  ].join('\n')

  return `${head}\n\n${clean}\n\n${foot}\n`
}

function parseArgs(argv) {
  const a = {}
  for (let i = 0; i < argv.length; i += 2) a[argv[i].replace(/^--/, '')] = argv[i + 1]
  return a
}

if (import.meta.url === pathToFileURL(process.argv[1] || '').href) {
  const a = parseArgs(process.argv.slice(2))
  if (!a.version || !a['body-file']) {
    process.stderr.write('usage: release-notes-wrap.mjs --version V --prev TAG --body-file F [--contributors a,b]\n')
    process.exit(2)
  }
  const list = (a.contributors || process.env.RELEASE_CONTRIBUTORS || '').split(/[,\s]+/).filter(Boolean)
  process.stdout.write(wrap({ version: a.version, prev: a.prev || '', body: fs.readFileSync(a['body-file'], 'utf8'), contributors: list }))
}
