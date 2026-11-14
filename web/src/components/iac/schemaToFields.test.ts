import { describe, expect, it } from 'vitest'
import { realSchema } from './iacTestSchema'
import {
  isSecretLikeName,
  labelFromName,
  schemaToFields,
} from './schemaToFields'

describe('schemaToFields', () => {
  it('models App with required metadata name and a service group', () => {
    const m = schemaToFields(realSchema, 'App')
    const byId = Object.fromEntries(m.fields.map((f) => [f.id, f]))
    expect(m.stability).toBe('stable')
    expect(byId['metadata.name']).toMatchObject({
      type: 'string',
      required: true,
      effectiveRequired: true,
    })
    expect(byId['metadata.name']?.pattern).toBeTruthy()
    expect(byId['metadata.labels']?.type).toBe('keyValue')
    expect(byId['spec.service']?.type).toBe('group')
    expect(byId['spec.service.port']).toMatchObject({
      type: 'number',
      integer: true,
      min: 1,
      max: 65535,
    })
    expect(byId['spec.service.image']?.description).toMatch(/image/i)
    expect(byId['spec.service.env']).toMatchObject({
      type: 'keyValue',
      secretRefs: true,
    })
    expect(byId['spec.service.bind_address']?.default).toBe('private')
    expect(byId['spec.service.bind_address']?.label).toBe('Bind address')
    expect(byId['spec.tags']?.type).toBe('stringList')
  })

  it('models Database with an engine enum that is required', () => {
    const m = schemaToFields(realSchema, 'Database')
    const engine = m.fields.find((f) => f.id === 'spec.engine')
    expect(engine).toMatchObject({
      type: 'enum',
      required: true,
      effectiveRequired: true,
    })
    expect(engine?.options).toContain('postgres')
    const seed = m.fields.find((f) => f.id === 'spec.version')
    expect(seed?.advanced).toBe(false)
  })

  it('reads beta stability from x-stability', () => {
    expect(schemaToFields(realSchema, 'Pipeline').stability).toBe('beta')
    expect(schemaToFields(realSchema, 'Tag').fields.map((f) => f.id)).toEqual([
      'metadata.name',
      'metadata.labels',
    ])
  })

  it('flags secret-like names and derives labels', () => {
    expect(isSecretLikeName('DB_PASSWORD')).toBe(true)
    expect(isSecretLikeName('apiToken')).toBe(true)
    expect(isSecretLikeName('port')).toBe(false)
    expect(labelFromName('ephemeralInPreviews')).toBe('Ephemeral in previews')
  })
})
