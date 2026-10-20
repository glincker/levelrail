import { describe, expect, it } from 'vitest'
import type { AppDetail } from '../types/appDetail'
import { formatChangeValue, patchApp, StaleFixError } from './diagnosisFix'

const app = {
  name: 'web',
  image: 'web:1',
  port: 3000,
  bind_address: 'private',
  strategy: 'recreate',
  replicas: 1,
  env: { A: '1' },
  resources: { memory_bytes: 268435456 },
  health: { readiness: { path: '/health' } },
} as unknown as AppDetail

describe('patchApp', () => {
  it('changes the port without mutating the input', () => {
    const next = patchApp(
      app,
      [{ field: 'port', from: '3000', to: '8080' }],
      {},
    )
    expect(next.port).toBe(8080)
    expect(app.port).toBe(3000)
  })

  it('raises the memory limit', () => {
    const next = patchApp(
      app,
      [
        {
          field: 'resources.memory_bytes',
          from: '268435456',
          to: '536870912',
        },
      ],
      {},
    )
    expect(next.resources?.memory_bytes).toBe(536870912)
  })

  it('changes the readiness path and sets env from input', () => {
    const next = patchApp(
      app,
      [
        { field: 'health.readiness.path', from: '/health', to: '/healthz' },
        { field: 'env.API_KEY', from: '', to: '', needs_input: true },
      ],
      { 'env.API_KEY': 'secret' },
    )
    expect(next.health?.readiness?.path).toBe('/healthz')
    expect(next.env).toEqual({ A: '1', API_KEY: 'secret' })
  })

  it('refuses stale changes', () => {
    expect(() =>
      patchApp(app, [{ field: 'port', from: '9999', to: '8080' }], {}),
    ).toThrow(StaleFixError)
    expect(() =>
      patchApp(
        app,
        [{ field: 'health.readiness.path', from: '/x', to: '/y' }],
        {},
      ),
    ).toThrow(StaleFixError)
    expect(() =>
      patchApp(app, [{ field: 'env.A', from: '', to: '', needs_input: true }], {
        'env.A': 'new',
      }),
    ).toThrow(StaleFixError)
  })

  it('requires a value for input changes and rejects unknown fields', () => {
    expect(() =>
      patchApp(
        app,
        [{ field: 'env.B', from: '', to: '', needs_input: true }],
        {},
      ),
    ).toThrow(/required/)
    expect(() =>
      patchApp(app, [{ field: 'image', from: '', to: 'x' }], {}),
    ).toThrow(/Unsupported/)
  })
})

describe('formatChangeValue', () => {
  it('shows memory in MiB', () => {
    expect(formatChangeValue('resources.memory_bytes', '536870912')).toBe(
      '512 MiB',
    )
    expect(formatChangeValue('port', '')).toBe('unset')
  })
})
