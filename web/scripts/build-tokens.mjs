#!/usr/bin/env node
import { readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const sourcePath = path.join(root, 'src/styles/tokens/glinr.tokens.json')
const tokensCssPath = path.join(root, 'src/styles/glinr-tokens.css')
const themeCssPath = path.join(root, 'src/styles/glinr-theme.css')
// Stable, unhashed URL so the pre-paint script can link it for OLED users only.
const oledCssPath = path.join(root, 'public/glinr.oled.css')
const materialCssPath = path.join(root, 'src/styles/tokens/glinr.material.css')

const HEADER =
  '/* GLINR design tokens, generated from tokens/glinr.tokens.json, do not edit. Run `npm run tokens`. */'

function isToken(node) {
  return node !== null && typeof node === 'object' && '$value' in node
}

export function collect(tree) {
  const tokens = []
  for (const [group, groupNode] of Object.entries(tree)) {
    if (group.startsWith('$')) continue
    for (const [key, node] of Object.entries(groupNode)) {
      if (key.startsWith('$')) continue
      if (!isToken(node)) throw new Error(`${group}.${key} is not a token`)
      const ext = node.$extensions?.glinr ?? {}
      tokens.push({
        path: `${group}.${key}`,
        name: `--glinr-${ext.name ?? `${group}-${key}`}`,
        value: node.$value,
        stability: ext.stability,
        chunk: ext.chunk,
        tailwind: ext.tailwind,
        description: node.$description,
      })
    }
  }
  return tokens
}

function resolve(value, byPath) {
  const alias = /^\{([^}]+)\}$/.exec(value)
  if (!alias) return value
  const target = byPath.get(alias[1])
  if (!target) throw new Error(`unknown alias ${value}`)
  return `var(${target.name})`
}

export function build(tree) {
  const tokens = collect(tree)
  const byPath = new Map(tokens.map((t) => [t.path, t]))
  const seen = new Set()
  for (const t of tokens) {
    if (seen.has(t.name)) throw new Error(`duplicate token name ${t.name}`)
    seen.add(t.name)
    if (t.stability !== 'stable' && t.stability !== 'experimental') {
      throw new Error(`${t.path} needs stability stable|experimental`)
    }
    if (!t.description) throw new Error(`${t.path} needs a $description`)
  }

  const bases = { base: [[], [], []], material: [[], [], []] }
  for (const t of tokens) {
    const [light, dark, oled] =
      bases[t.chunk === 'material' ? 'material' : 'base']
    const modal =
      typeof t.value === 'object' && t.value !== null && 'light' in t.value
    const lightValue = modal ? t.value.light : t.value
    light.push(`  ${t.name}: ${resolve(lightValue, byPath)};`)
    if (modal && t.value.dark !== t.value.light) {
      dark.push(`  ${t.name}: ${resolve(t.value.dark, byPath)};`)
    }
    // .oled always rides on .dark, so only values that differ from dark are emitted.
    if (modal && t.value.oled !== undefined && t.value.oled !== t.value.dark) {
      oled.push(`  ${t.name}: ${resolve(t.value.oled, byPath)};`)
    }
  }

  const rules = (set, withOled) =>
    `:root, [data-mode="light"] {\n${set[0].join('\n')}\n}\n\n.dark, [data-mode="dark"] {\n${set[1].join('\n')}\n}\n${withOled ? `\n${oledRule(set[2])}` : ''}`
  // Doubled selectors beat .dark even when this sheet is linked before the bundle.
  const oledRule = (lines) =>
    `.dark.oled, [data-mode="oled"][data-mode] {\n${lines.join('\n')}\n}\n`

  const tokensCss = `${HEADER}\n\n${rules(bases.base, false)}`
  const oledCss = `${HEADER}\n\n${oledRule(bases.base[2])}`
  const materialCss = `${HEADER}\n\n${rules(bases.material, true)}`

  const mapped = tokens.filter((t) => t.tailwind)
  const themeCss = `${HEADER}\n\n@theme inline {\n${mapped
    .map((t) => `  --${t.tailwind}: var(${t.name});`)
    .join('\n')}\n}\n`

  return { tokens, tokensCss, themeCss, oledCss, materialCss }
}

function main() {
  const tree = JSON.parse(readFileSync(sourcePath, 'utf8'))
  const { tokens, tokensCss, themeCss, oledCss, materialCss } = build(tree)
  writeFileSync(tokensCssPath, tokensCss)
  writeFileSync(themeCssPath, themeCss)
  writeFileSync(oledCssPath, oledCss)
  writeFileSync(materialCssPath, materialCss)
  console.log(`glinr tokens: ${tokens.length} tokens written`)
}

if (
  process.argv[1] &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)
) {
  main()
}
