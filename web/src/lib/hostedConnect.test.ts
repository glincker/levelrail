import { describe, expect, it } from 'vitest'
import {
  formFromProvider,
  hostedRequest,
  suggestHostedName,
  tlsOutcome,
} from './hostedConnect'
import type { ExternalProvider } from '../types/externalDatabase'

const neon: ExternalProvider = {
  id: 'neon',
  label: 'Neon',
  brand: 'neon',
  engines: ['postgres'],
  tls_mode: 'verify-full',
  host_hint: 'ep-name.region.aws.neon.tech',
  docs_url: 'https://example.test',
}

describe('hostedConnect', () => {
  it('prefills engine, port and TLS from the preset', () => {
    expect(formFromProvider(neon)).toMatchObject({
      engine: 'postgres',
      port: '5432',
      tls: 'verify-full',
    })
    expect(formFromProvider(null).engine).toBe('postgres')
  })

  it('suggests a name like the server does', () => {
    expect(
      suggestHostedName('neon', 'ep-example-123.us-east-2.aws.neon.tech'),
    ).toBe('neon-ep-example-123')
    expect(suggestHostedName('generic', 'DB.Example.com')).toBe('db')
  })

  it('sends the string and the provider, and omits empty fields', () => {
    const req = hostedRequest(
      formFromProvider(neon),
      'neon',
      ' postgres://u:p@h/db ',
      false,
    )
    expect(req.url).toBe('postgres://u:p@h/db')
    expect(req.password).toBeUndefined()
    expect(req.save_anyway).toBeUndefined()
  })

  it('reads TLS from the mode the probe ran with', () => {
    const ok = { status: 'reachable' as const, latency_ms: 4 }
    expect(tlsOutcome('verify-full', ok)).toBe('verified')
    expect(tlsOutcome('require', ok)).toBe('unverified')
    expect(tlsOutcome('prefer', ok)).toBe('notChecked')
    expect(tlsOutcome('disable', ok)).toBe('off')
  })
})
