// Builds the /help manifest (titles, headings, README-derived sidebar
// categories) from /docs at build/dev/test time via two virtual modules,
// the same "never commit a generated file" convention this project
// already follows for routeTree.gen.ts. A previous version of this
// feature committed the manifest as a real src/generated/*.ts file;
// that tripped SonarCloud's duplication gate (its ~30 pages' worth of
// repetitive {id, text, level} object literals reads as copy-pasted
// code to a token-based detector), so it's a build artifact instead,
// invisible to source analysis the same way routeTree.gen.ts already is.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const docsDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../docs',
)

// Must match web/src/lib/slugify.ts exactly.
function slugify(text) {
  return text
    .toLowerCase()
    .trim()
    .replace(/[`~!@#$%^&*()+={}[\]|\\:;"'<>,.?/]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
}

function uniqueSlug(text, seen) {
  const base = slugify(text)
  const count = seen.get(base) ?? 0
  seen.set(base, count + 1)
  return count === 0 ? base : `${base}-${count}`
}

// Must match web/src/lib/docsPaths.ts exactly.
function filePathToRoutePath(relFile) {
  if (relFile === 'README.md') return '/'
  return `/${relFile.replace(/\.md$/, '')}`
}

function listMarkdownFiles(dir, base = '') {
  const out = []
  for (const entry of readdirSync(dir)) {
    if (entry.startsWith('.')) continue
    const full = path.join(dir, entry)
    const rel = base ? `${base}/${entry}` : entry
    if (statSync(full).isDirectory()) {
      out.push(...listMarkdownFiles(full, rel))
    } else if (entry.endsWith('.md')) {
      out.push(rel)
    }
  }
  return out
}

function stripFrontmatter(text) {
  if (!text.startsWith('---\n') && !text.startsWith('---\r\n')) return text
  const end = text.indexOf('\n---', 4)
  if (end === -1) return text
  const rest = text.slice(end + 4)
  return rest.startsWith('\n') ? rest.slice(1) : rest
}

function extractPage(relFile) {
  const raw = readFileSync(path.join(docsDir, relFile), 'utf8')
  const body = stripFrontmatter(raw)
  const lines = body.split('\n')
  let title
  const headings = []
  const seen = new Map()
  for (const line of lines) {
    const h1 = /^#\s+(.+)$/.exec(line)
    if (h1 && !title) {
      title = h1[1].trim()
      continue
    }
    const h = /^(#{2,3})\s+(.+)$/.exec(line)
    if (h) {
      const level = h[1].length
      const text = h[2].trim()
      headings.push({ id: uniqueSlug(text, seen), text, level })
    }
  }
  return { file: relFile, title: title ?? relFile, headings }
}

function extractCategories(readmeText, pagesByPath) {
  const body = stripFrontmatter(readmeText)
  const lines = body.split('\n')
  const categories = []
  let current = null
  for (const line of lines) {
    const heading = /^###\s+(.+)$/.exec(line)
    if (heading) {
      current = { name: heading[1].trim(), docs: [] }
      categories.push(current)
      continue
    }
    if (!current) continue
    const link = /\[([^\]]+)\]\(([^)]+?)\)/.exec(line)
    if (!link) continue
    const [, , target] = link
    if (!target.endsWith('.md')) continue
    const relFile = path.posix.normalize(target)
    const routePath = filePathToRoutePath(relFile)
    const page = pagesByPath.get(routePath)
    if (!page) continue
    // README.md's own link text is the bare filename (a GitHub-preview
    // convention), not a human title; the sidebar wants the page's real
    // title (its own H1), not "getting-started.md".
    current.docs.push({ path: routePath, title: page.title })
  }
  return categories
}

export function buildDocsManifest() {
  const files = listMarkdownFiles(docsDir).filter((f) => f !== 'index.md')

  const pages = {}
  for (const relFile of files) {
    pages[filePathToRoutePath(relFile)] = extractPage(relFile)
  }

  const pagesByPath = new Map(Object.entries(pages))
  const readmeText = readFileSync(path.join(docsDir, 'README.md'), 'utf8')
  const categories = extractCategories(readmeText, pagesByPath)

  return { categories, pages }
}

const MANIFEST_ID = 'virtual:docs-manifest'
const RESOLVED_MANIFEST_ID = `\0${MANIFEST_ID}`
const PATH_INDEX_ID = 'virtual:docs-path-index'
const RESOLVED_PATH_INDEX_ID = `\0${PATH_INDEX_ID}`

// Registered in both vite.config.ts (build/dev) and vitest.config.ts
// (tests), so every consumer of these two specifiers works identically
// everywhere: HelpLink/AppSidebar import the tiny path-index eagerly,
// while /help's own routes load the full manifest through a dynamic
// import, keeping it out of the always-loaded main bundle.
export function docsManifestPlugin() {
  return {
    name: 'docs-manifest',
    resolveId(id) {
      if (id === MANIFEST_ID) return RESOLVED_MANIFEST_ID
      if (id === PATH_INDEX_ID) return RESOLVED_PATH_INDEX_ID
      return undefined
    },
    load(id) {
      if (id === RESOLVED_MANIFEST_ID) {
        return `export default ${JSON.stringify(buildDocsManifest())}`
      }
      if (id === RESOLVED_PATH_INDEX_ID) {
        const manifest = buildDocsManifest()
        return `export default new Set(${JSON.stringify(Object.keys(manifest.pages))})`
      }
      return undefined
    },
  }
}
