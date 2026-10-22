import { describe, expect, it } from 'vitest'
import {
  appSlug,
  computeTemplateSummary,
  parseMemory,
  primaryVersion,
} from './templateSummary'
import {
  envFieldKind,
  groupEnvPrompts,
  validateTemplateForm,
} from './templateForm'

const GIB = 1024 ** 3

describe('computeTemplateSummary', () => {
  it('collects ports, volumes, a database and backups from the definition', () => {
    const s = computeTemplateSummary({
      compose: 'services:\n  ghost:\n    mem_limit: 512m\n    cpus: 0.5\n',
      recommendedMemoryBytes: GIB,
      services: [
        { key: 'ghost', image: 'ghost:5.8', port: 2368, volumes: ['content'] },
        { key: 'db', image: 'mysql:8', volumes: ['dbdata', 'content'] },
      ],
    })
    expect(s.ports).toEqual([2368])
    expect(s.volumes).toEqual(['content', 'dbdata'])
    expect(s.databases).toEqual(['mysql'])
    expect(s.hasBackups).toBe(true)
    expect(s.memoryBytes).toBe(512 * 1024 ** 2)
    expect(s.cpus).toBe(0.5)
    expect(s.serviceCount).toBe(2)
  })

  it('falls back to the recommended memory and reports nothing for a bare service', () => {
    const s = computeTemplateSummary({
      compose: 'services:\n  web:\n    image: nginx\n',
      recommendedMemoryBytes: 256 * 1024 ** 2,
      services: [{ key: 'web', image: 'nginx:1.27' }],
    })
    expect(s.memoryBytes).toBe(256 * 1024 ** 2)
    expect(s.cpus).toBeUndefined()
    expect(s.volumes).toEqual([])
    expect(s.databases).toEqual([])
    expect(s.hasBackups).toBe(false)
  })

  it('parses sizes and versions', () => {
    expect(parseMemory('1.5g')).toBe(1.5 * GIB)
    expect(parseMemory('nope')).toBeUndefined()
    expect(primaryVersion([{ key: 'a', image: 'ghost:5.8' }])).toBe('5.8')
    expect(
      primaryVersion([{ key: 'a', image: 'ghost:latest' }]),
    ).toBeUndefined()
    expect(appSlug('ghost', 'Ghost CMS')).toBe('ghost-cms')
  })
})

describe('env form classification', () => {
  const prompts = [
    { key: 'A', required: true },
    { key: 'B', required: true, default: 'x' },
    { key: 'C', generate: 'password' as const, required: true },
    { key: 'D' },
  ]

  it('sorts into required, generated and optional', () => {
    expect(envFieldKind(prompts[0]!)).toBe('required')
    expect(envFieldKind(prompts[1]!)).toBe('optional')
    expect(envFieldKind(prompts[2]!)).toBe('generated')
    const g = groupEnvPrompts(prompts)
    expect(g.required.map((p) => p.key)).toEqual(['A'])
    expect(g.generated.map((p) => p.key)).toEqual(['C'])
    expect(g.optional.map((p) => p.key)).toEqual(['B', 'D'])
  })

  it('validates required values, the app name and custom domains', () => {
    const base = { name: '', env: {}, customDomain: false, domains: {} }
    expect(validateTemplateForm(prompts, base)).toEqual({ 'env:A': 'required' })
    expect(
      validateTemplateForm(prompts, {
        ...base,
        name: 'Bad Name',
        env: { A: 'ok' },
        customDomain: true,
        domains: { web: 'nope' },
      }),
    ).toEqual({ name: 'name', 'domain:web': 'domain' })
    expect(
      validateTemplateForm(prompts, {
        ...base,
        env: { A: 'ok' },
        customDomain: true,
        domains: { web: 'app.example.com' },
      }),
    ).toEqual({})
  })
})
