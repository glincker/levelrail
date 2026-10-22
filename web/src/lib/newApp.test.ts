import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApplyStepError,
  applyNewApp,
  editsFromAnalysis,
  missingRequiredEnv,
  plannedChanges,
  validateEdits,
} from './newApp'
import { jsonResponse, nextAnalysis } from '../test/newAppFixtures'
import type { GitRepoSourceValue } from '../components/GitRepoSourcePicker'

const source: GitRepoSourceValue = {
  provider: 'github',
  repoUrl: 'https://github.com/acme/shop.git',
  branch: 'main',
  providerRef: { kind: 'github', owner: 'acme', repo: 'shop' },
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('editsFromAnalysis', () => {
  it('pre-fills every field from the analysis', () => {
    const e = editsFromAnalysis(nextAnalysis)
    expect(e).toMatchObject({
      name: 'shop',
      port: 3000,
      healthPath: '/',
      memoryMb: 768,
      buildType: 'railpack',
      domain: 'shop.apps.example.com',
    })
    expect(e.databases).toEqual([
      expect.objectContaining({
        enabled: true,
        engine: 'postgres',
        name: 'shop-postgres',
        envVar: 'DATABASE_URL',
      }),
    ])
  })
})

describe('missing and planned', () => {
  it('requires secrets but not database-provided keys', () => {
    const e = editsFromAnalysis(nextAnalysis)
    expect(missingRequiredEnv(nextAnalysis, e)).toEqual(['NEXTAUTH_SECRET'])
    e.env.NEXTAUTH_SECRET = 'abc'
    expect(missingRequiredEnv(nextAnalysis, e)).toEqual([])
  })

  it('lists app, source, database, domain and required env as changes', () => {
    const e = editsFromAnalysis(nextAnalysis)
    const kinds = plannedChanges(nextAnalysis, e).map((c) => c.kind)
    expect(kinds).toEqual(['app', 'source', 'database', 'domain', 'env'])
    e.databases[0]!.enabled = false
    expect(plannedChanges(nextAnalysis, e).map((c) => c.kind)).not.toContain(
      'database',
    )
  })

  it('flags an invalid name and port', () => {
    const e = { ...editsFromAnalysis(nextAnalysis), name: 'Bad Name', port: 0 }
    expect(validateEdits(e)).toEqual(['name', 'port'])
  })
})

describe('applyNewApp', () => {
  function mockApi(failOn?: string) {
    const calls: { key: string; body: unknown }[] = []
    vi.spyOn(globalThis, 'fetch').mockImplementation((input, init) => {
      const method = init?.method ?? 'GET'
      const key = `${method} ${urlOf(input)}`
      const body: unknown =
        typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
      calls.push({ key, body })
      if (failOn && key === failOn) {
        return Promise.resolve(jsonResponse({ error: 'boom' }, 500))
      }
      if (key === 'GET /api/v1/databases/shop-postgres') {
        return Promise.resolve(jsonResponse({ error: 'not found' }, 404))
      }
      if (key === 'POST /api/v1/apps/shop/builds') {
        return Promise.resolve(jsonResponse({ id: 'att1' }, 202))
      }
      return Promise.resolve(jsonResponse(body ?? {}, 201))
    })
    return calls
  }

  it('creates database, app, connection, source and build in order', async () => {
    const calls = mockApi()
    const e = editsFromAnalysis(nextAnalysis)
    e.env = { NEXTAUTH_SECRET: 's3cret', NEXT_PUBLIC_URL: 'https://x.test' }

    const result = await applyNewApp(nextAnalysis, source, e)

    expect(result).toEqual({ appName: 'shop', deployId: 'att1', options: [] })
    expect(calls.map((c) => c.key)).toEqual([
      'GET /api/v1/databases/shop-postgres',
      'POST /api/v1/databases',
      'POST /api/v1/apps',
      'POST /api/v1/apps/shop/connections',
      'POST /api/v1/github-app/repos/acme/shop/use-as-source',
      'POST /api/v1/apps/shop/builds',
    ])
    const app = calls[2]!.body as Record<string, unknown>
    expect(app).toMatchObject({
      name: 'shop',
      port: 3000,
      domains: ['shop.apps.example.com'],
      env: { NEXT_PUBLIC_URL: 'https://x.test' },
      secret_env: ['NEXTAUTH_SECRET'],
      secrets: { NEXTAUTH_SECRET: 's3cret' },
      resources: { memory_bytes: 768 * 1024 * 1024 },
    })
    expect(JSON.stringify(calls[5]!.body)).not.toContain('s3cret')
  })

  it('names the failing step and keeps whether the app exists', async () => {
    mockApi('POST /api/v1/apps/shop/connections')
    const e = editsFromAnalysis(nextAnalysis)
    await expect(applyNewApp(nextAnalysis, source, e)).rejects.toMatchObject({
      step: 'connection',
      appCreated: true,
    })
    mockApi('POST /api/v1/databases')
    await expect(applyNewApp(nextAnalysis, source, e)).rejects.toBeInstanceOf(
      ApplyStepError,
    )
  })

  it('skips app creation when resuming', async () => {
    const calls = mockApi()
    await applyNewApp(
      nextAnalysis,
      source,
      editsFromAnalysis(nextAnalysis),
      true,
    )
    expect(calls.map((c) => c.key)).not.toContain('POST /api/v1/apps')
  })
})
