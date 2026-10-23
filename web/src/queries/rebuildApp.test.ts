import { afterEach, describe, expect, it, vi } from 'vitest'
import { rebuildApp } from './rebuildApp'

const gitSource = {
  service_name: 'web',
  repo_url: 'https://example.com/web.git',
  branch: 'main',
  build_type: 'dockerfile',
  build_path: 'Dockerfile',
}

function stubFetch() {
  const calls: { url: string; body?: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, body: init?.body as string | undefined })
      const payload = url.endsWith('/git-source') ? gitSource : { id: 'dep_1' }
      return Promise.resolve(new Response(JSON.stringify(payload)))
    }),
  )
  return calls
}

describe('rebuildApp', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each([
    { name: 'with cache', noCache: false },
    { name: 'without cache', noCache: true },
  ])('$name', async ({ noCache }) => {
    const calls = stubFetch()
    const res = await rebuildApp('web', { noCache })
    expect(res.id).toBe('dep_1')
    const call = calls[1]
    expect(call?.url).toBe('/api/v1/apps/web/builds')
    const body = JSON.parse(call?.body ?? '{}') as Record<string, unknown>
    expect(body.repo_url).toBe(gitSource.repo_url)
    expect(body.ref).toBe('main')
    expect(body.no_cache).toBe(noCache ? true : undefined)
  })
})
