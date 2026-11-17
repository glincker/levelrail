import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { render } from './render-mcp-server-json.mjs'

const read = (p) => readFileSync(new URL(p, import.meta.url), 'utf8')
const template = JSON.parse(read('../server.json'))

test('render sets the version everywhere from the tag', () => {
  const out = render(template, 'v2.3.4')
  assert.equal(out.version, '2.3.4')
  assert.equal(out.packages[0].version, '2.3.4')
  assert.equal(out.packages[0].registryType, 'npm')
  assert.equal(out.packages[0].transport.type, 'stdio')
})

test('render refuses prerelease and malformed tags', () => {
  assert.throws(() => render(template, 'v1.4.0-rc.1'))
  assert.throws(() => render(template, 'main'))
})

test('the token variable is required and secret', () => {
  const token = template.packages[0].environmentVariables.find((e) => e.name === 'APP_API_TOKEN')
  assert.equal(token.isRequired, true)
  assert.equal(token.isSecret, true)
})

test('the generated package carries the same mcpName and bin as server.json', () => {
  const script = read('./build-mcp-npm-packages.sh')
  assert.ok(script.includes(`"mcpName": "${template.name}"`))
  assert.ok(script.includes(`"name": "${template.packages[0].identifier}"`))
  assert.ok(script.includes('"bin": { "levelrail-mcp":'))
})
