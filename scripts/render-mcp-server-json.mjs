#!/usr/bin/env node
// Renders server.json for a release tag so the committed file never goes stale.
// Usage: node scripts/render-mcp-server-json.mjs <tag> [out-file]  (default: stdout)
import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const root = new URL('..', import.meta.url)
const MAX_DESCRIPTION = 100
const TAG = /^v?(\d+\.\d+\.\d+)$/

export function render(template, tag) {
  const m = TAG.exec(tag)
  if (!m) throw new Error(`not a stable release tag: ${tag}`)
  const version = m[1]
  const out = structuredClone(template)
  out.version = version
  for (const pkg of out.packages ?? []) pkg.version = version
  if (!out.name?.startsWith('io.github.glincker/')) throw new Error(`name outside the glincker namespace: ${out.name}`)
  if (out.description.length > MAX_DESCRIPTION) throw new Error(`description over ${MAX_DESCRIPTION} characters`)
  const secret = out.packages[0].environmentVariables.find((e) => e.isSecret)
  if (!secret?.isRequired) throw new Error('the API token variable must be required and secret')
  return out
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const [tag, outFile] = process.argv.slice(2)
  if (!tag) {
    console.error('usage: render-mcp-server-json.mjs <tag> [out-file]')
    process.exit(2)
  }
  const template = JSON.parse(readFileSync(new URL('server.json', root), 'utf8'))
  const text = JSON.stringify(render(template, tag), null, 2) + '\n'
  if (outFile) writeFileSync(outFile, text)
  else process.stdout.write(text)
}
