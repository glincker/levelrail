import { describe, expect, it } from 'vitest'
import schema from '../../../docs/schemas/resources.schema.json'
import { resolveFieldHelp, yamlPathAtLine } from './schemaHelp'

describe('schemaHelp', () => {
  it('maps a yaml line to its key path and kind', () => {
    const lines = [
      'kind: App',
      'spec:',
      '  service:',
      '    health:',
      '      readyTimeout: 60s',
    ]
    expect(yamlPathAtLine(lines, 4)).toEqual({
      kind: 'App',
      path: ['spec', 'service', 'health', 'readyTimeout'],
    })
  })

  it('resolves description and default from the schema', () => {
    const help = resolveFieldHelp(schema, 'App', [
      'spec',
      'service',
      'health',
      'readyTimeout',
    ])
    expect(help?.description).toMatch(/readiness/i)
    expect(help?.defaultValue).toBe('"60s"')
    expect(resolveFieldHelp(schema, 'App', ['spec', 'nope'])).toBeNull()
  })
})
