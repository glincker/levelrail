import { mkdirSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'
import type { SiteConfig } from 'vitepress'

const SKIP_DIRS = new Set(['node_modules', '.vitepress', 'public'])

function stripFrontmatter(content: string): string {
  return content.replace(/^---\n[\s\S]*?\n---\n/, '')
}

function findMarkdownFiles(dir: string, root: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name.startsWith('.')) continue
    const full = join(dir, entry.name)
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) findMarkdownFiles(full, root, out)
    } else if (entry.name.endsWith('.md')) {
      out.push(relative(root, full))
    }
  }
  return out
}

// Serves the plain-markdown source of every page at <page>.md alongside
// its rendered HTML -- what PageActions.vue's "Copy Markdown" and "View
// as Markdown" read, and a convention (Mintlify/Fumadocs/etc.) AI tools
// increasingly check for before falling back to scraping rendered HTML.
export function writeRawMarkdown(config: SiteConfig) {
  for (const file of findMarkdownFiles(config.srcDir, config.srcDir)) {
    const dest = join(config.outDir, file)
    mkdirSync(dirname(dest), { recursive: true })
    writeFileSync(dest, stripFrontmatter(readFileSync(join(config.srcDir, file), 'utf-8')))
  }
}
