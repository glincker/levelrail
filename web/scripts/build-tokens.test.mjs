import { readFileSync, readdirSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { build } from './build-tokens.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const tree = JSON.parse(
  readFileSync(path.join(root, 'src/styles/tokens/glinr.tokens.json'), 'utf8'),
)
const { tokens, tokensCss, themeCss } = build(tree)
const names = new Set(tokens.map((t) => t.name))

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (/\.(css|tsx?|mdx?)$/.test(entry) && !entry.endsWith('.gen.ts'))
      out.push(full)
  }
  return out
}

describe('glinr design tokens', () => {
  it('committed CSS matches the generator output', () => {
    const committed = readFileSync(
      path.join(root, 'src/styles/glinr-tokens.css'),
      'utf8',
    )
    expect(committed).toBe(tokensCss)
    const theme = readFileSync(
      path.join(root, 'src/styles/glinr-theme.css'),
      'utf8',
    )
    expect(theme).toBe(themeCss)
  })

  it('declares every token name in the generated CSS', () => {
    for (const name of names) expect(tokensCss).toContain(`${name}:`)
  })

  it('keeps every legacy variable name', () => {
    const legacy = [
      'bg',
      'surface',
      'border',
      'radius',
      'content-max',
      'content-max-wide',
      'page-gap',
      'tile-size',
      'hero-tile-size',
      'font-display',
      'font-eyebrow',
      'display-weight',
      'showcase-bg',
      'showcase-surface',
      'showcase-text',
      'glow-violet',
      'glow-teal',
      'glow-lime',
      'highlight',
      'shadow-color',
      'elev-header',
      'elev-card',
      'elev-popover',
      'elev-dialog',
    ]
    for (const name of legacy) expect(names.has(`--glinr-${name}`)).toBe(true)
  })

  it('only references --glinr-* variables that exist in the source', () => {
    const missing = []
    for (const file of walk(path.join(root, 'src'))) {
      const text = readFileSync(file, 'utf8')
      for (const match of text.matchAll(/var\((--glinr-[a-z0-9-]+)/g)) {
        if (!names.has(match[1]))
          missing.push(`${path.relative(root, file)}: ${match[1]}`)
      }
    }
    expect(missing).toEqual([])
  })
})
