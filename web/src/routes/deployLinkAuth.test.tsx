import { QueryClient } from '@tanstack/react-query'
import { createMemoryHistory, createRouter } from '@tanstack/react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearStoredUsername, setStoredUsername } from '../lib/authStore'
import { goAfterLogin } from '../queries/auth'
import { Route as loginRoute } from './login'
import { routeTree } from '../routeTree.gen'

const DEPLOY_PATH = '/templates?deploy=uptime-kuma'

function makeRouter(entry: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [entry] }),
  })
  return router
}

beforeEach(() => {
  clearStoredUsername()
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response('{}', { status: 404 }))),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  clearStoredUsername()
  window.history.replaceState(null, '', '/')
})

describe('deploy link through the sign-in redirect', () => {
  it('(a) logged out keeps the deploy query on the login redirect', async () => {
    const router = makeRouter(DEPLOY_PATH)
    await router.load()
    expect(router.state.location.pathname).toBe('/login')
    expect(router.state.location.search).toMatchObject({
      redirect: DEPLOY_PATH,
    })
  })

  it('(a) returns to the deploy link after sign-in', async () => {
    const router = makeRouter(DEPLOY_PATH)
    await router.load()
    window.history.replaceState(null, '', router.state.location.href)
    setStoredUsername('admin')
    goAfterLogin(router.navigate, router)
    await vi.waitFor(() =>
      expect(router.state.location.pathname).toBe('/templates'),
    )
    expect(router.state.location.search).toMatchObject({
      deploy: 'uptime-kuma',
    })
  })

  it('(b) first-run setup keeps the deploy query too', async () => {
    const router = makeRouter(`${DEPLOY_PATH}#setup=tok123`)
    await router.load()
    expect(router.state.location.pathname).toBe('/login')
    expect(router.state.location.search).toMatchObject({
      redirect: expect.stringContaining('deploy=uptime-kuma') as string,
    })
    expect(router.state.location.hash).toContain('setup=tok123')
  })

  it('(e) an email-change style hash survives sign-in', async () => {
    const router = makeRouter('/settings/security#token=abc123')
    await router.load()
    expect(router.state.location.pathname).toBe('/login')
    expect(router.state.location.hash).toBe('token=abc123')
    window.history.replaceState(null, '', router.state.location.href)
    setStoredUsername('admin')
    goAfterLogin(router.navigate, router)
    await vi.waitFor(() =>
      expect(router.state.location.pathname).toBe('/settings/security'),
    )
    expect(router.state.location.hash).toBe('token=abc123')
  })

  it('(f) the first-run setup token is not carried past sign-in', async () => {
    window.history.replaceState(
      null,
      '',
      '/login?redirect=%2Ftemplates#setup=t',
    )
    setStoredUsername('admin')
    const router = makeRouter('/login')
    await router.load()
    goAfterLogin(router.navigate, router)
    await vi.waitFor(() =>
      expect(router.state.location.pathname).toBe('/templates'),
    )
    expect(router.state.location.hash).toBe('')
  })

  it('(g) an unsafe return path with a hash is still rejected', async () => {
    window.history.replaceState(
      null,
      '',
      '/login?redirect=%2F%2Fevil.example#token=x',
    )
    setStoredUsername('admin')
    const router = makeRouter('/login')
    await router.load()
    goAfterLogin(router.navigate, router)
    await vi.waitFor(() =>
      expect(router.state.location.pathname).not.toBe('/login'),
    )
    expect(router.state.location.href).not.toContain('evil.example')
  })

  it('(c) logged in stays on the deploy link', async () => {
    setStoredUsername('admin')
    const router = makeRouter(DEPLOY_PATH)
    await router.load()
    expect(router.state.location.pathname).toBe('/templates')
    expect(router.state.location.search).toMatchObject({
      deploy: 'uptime-kuma',
    })
  })

  it('(d) an unknown id is still carried through the redirect', async () => {
    const router = makeRouter('/templates?deploy=no-such-template')
    await router.load()
    expect(router.state.location.pathname).toBe('/login')
    expect(router.state.location.search).toMatchObject({
      redirect: '/templates?deploy=no-such-template',
    })
  })

  it.each([
    ['absolute url', 'https://evil.example/x'],
    ['protocol relative', '//evil.example/x'],
    ['javascript scheme', 'javascript:alert(1)'],
    ['backslash host', '/\\evil.example'],
    ['tab smuggled host', '/\t/evil.example'],
  ])('login search validation drops an unsafe redirect (%s)', (_n, bad) => {
    const validate = loginRoute.options.validateSearch as (
      search: Record<string, unknown>,
    ) => Record<string, unknown>
    expect(validate({ redirect: bad })).not.toHaveProperty('redirect')
  })
})
