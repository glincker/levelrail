import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import type { SiteConfig } from 'vitepress'

// changelog/ is a dynamic per-release route, not one markdown file, and
// its full history would dwarf the rest of this file for little value to
// an LLM reading it once; llms.txt still links to it.
const SKIP_SLUGS = new Set(['changelog/'])

function stripFrontmatter(content: string): string {
  return content.replace(/^---\n[\s\S]*?\n---\n/, '')
}

// Derived from public/llms.txt rather than a second hand-maintained
// list: llms.txt is the curated index, this is the full text of the same
// pages in the same order, so the two can't drift apart.
export function buildLlmsFullTxt(config: SiteConfig, siteUrl: string) {
  const llmsTxt = readFileSync(join(config.srcDir, 'public', 'llms.txt'), 'utf-8')
  const header = llmsTxt.split('\n## ')[0].trim()

  const seen = new Set<string>()
  const slugs = [...llmsTxt.matchAll(/https:\/\/levelrail\.com\/([a-zA-Z0-9_/-]*)/g)]
    .map((m) => m[1])
    .filter((slug) => !SKIP_SLUGS.has(slug) && !seen.has(slug) && seen.add(slug))

  const sections = slugs
    .map((slug) => {
      const path = slug === '' ? 'index' : slug.replace(/\/$/, '/index')
      try {
        const body = stripFrontmatter(readFileSync(join(config.srcDir, `${path}.md`), 'utf-8')).trim()
        return `<!-- ${siteUrl}/${slug} -->\n\n${body}`
      } catch {
        return null
      }
    })
    .filter((section): section is string => section !== null)

  const full = `${header}\n\n---\n\n${sections.join('\n\n---\n\n')}\n`
  writeFileSync(join(config.outDir, 'llms-full.txt'), full)
}
