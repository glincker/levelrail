import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StaticRoutesI18n } from '../../test/staticRoutesI18n'
import { StaticRoutesPanel } from './StaticRoutesPanel'

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response
}

function mockFetch(routes: Record<string, () => Response>) {
  const fn = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : input.url
    const key = `${init?.method ?? 'GET'} ${url}`
    const hit = Object.entries(routes).find(([k]) => key.startsWith(k))
    return hit
      ? Promise.resolve(hit[1]())
      : Promise.reject(new Error(`unexpected fetch: ${key}`))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <StaticRoutesI18n>{node}</StaticRoutesI18n>
    </QueryClientProvider>,
  )
}

const ROUTES = {
  version: 1,
  published: true,
  rules: [
    { source: '_redirects:1', from: '/old', to: '/new', status: 301 },
    {
      source: 'app.yaml routes[0]',
      from: '/*',
      to: '/index.html',
      status: 200,
      force: true,
    },
  ],
  headers: [
    {
      source: '_headers:1',
      path: '/*',
      set: [{ name: 'X-Frame-Options', value: 'DENY' }],
    },
  ],
  options: { spa: true },
  problems: [
    {
      source: '_redirects',
      line: 4,
      severity: 'error',
      message: 'missing the destination after "/x"',
    },
  ],
}

afterEach(() => vi.unstubAllGlobals())

describe('StaticRoutesPanel', () => {
  it('shows the parsed rules, headers and the problems that were skipped', async () => {
    mockFetch({ 'GET /api/v1/apps/docs/static-routes': () => json(ROUTES) })
    wrap(<StaticRoutesPanel app="docs" />)
    expect(await screen.findByText('/old')).toBeTruthy()
    expect(screen.getByText('/new')).toBeTruthy()
    expect(screen.getByText('forced')).toBeTruthy()
    expect(screen.getByText('X-Frame-Options: DENY')).toBeTruthy()
    expect(screen.getByText(/missing the destination/)).toBeTruthy()
    expect(screen.getByText('line 4', { exact: false })).toBeTruthy()
  })

  it('explains a site published before routing rules existed', async () => {
    mockFetch({
      'GET /api/v1/apps/docs/static-routes': () =>
        json({
          ...ROUTES,
          published: false,
          rules: [],
          headers: [],
          problems: [],
        }),
    })
    wrap(<StaticRoutesPanel app="docs" />)
    expect(
      await screen.findByText('Published before routing rules existed'),
    ).toBeTruthy()
    expect(screen.getByText('No redirect or rewrite rules')).toBeTruthy()
  })

  it('says so when the app is not a static site', async () => {
    mockFetch({
      'GET /api/v1/apps/web/static-routes': () =>
        json({ error: 'static site not found' }, 404),
    })
    wrap(<StaticRoutesPanel app="web" />)
    expect(
      await screen.findByText('This app is not a static site'),
    ).toBeTruthy()
  })

  it('tests a URL and shows which rule answered', async () => {
    const fn = mockFetch({
      'GET /api/v1/apps/docs/static-routes': () => json(ROUTES),
      'POST /api/v1/apps/docs/static-routes/test': () =>
        json({
          kind: 'redirect',
          status: 301,
          location: '/new',
          rule: '_redirects:1',
          headers: [],
          remove_headers: [],
          trace: ['_redirects:1: redirect 301 to /new'],
        }),
    })
    wrap(<StaticRoutesPanel app="docs" />)
    await screen.findByText('/old')
    fireEvent.change(screen.getByLabelText('Path'), {
      target: { value: '/old' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Test' }))
    expect(await screen.findByText('Redirects to /new')).toBeTruthy()
    expect(screen.getByText('Rule: _redirects:1')).toBeTruthy()
    const post = fn.mock.calls.find(([, init]) => init?.method === 'POST')
    expect(
      JSON.parse(typeof post?.[1]?.body === 'string' ? post[1].body : '{}'),
    ).toEqual({
      path: '/old',
      query: '',
    })
  })

  it('refuses a test path without a leading slash', async () => {
    mockFetch({ 'GET /api/v1/apps/docs/static-routes': () => json(ROUTES) })
    wrap(<StaticRoutesPanel app="docs" />)
    await screen.findByText('/old')
    fireEvent.change(screen.getByLabelText('Path'), {
      target: { value: 'old' },
    })
    expect(screen.getByText('Enter a path that starts with /')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Test' })).toHaveProperty(
      'disabled',
      true,
    )
  })

  it('checks pasted rules without deploying', async () => {
    mockFetch({
      'GET /api/v1/apps/docs/static-routes': () => json(ROUTES),
      'POST /api/v1/apps/docs/static-routes/lint': () =>
        json({
          rules: [],
          headers: [],
          problems: [
            {
              source: '_redirects',
              line: 1,
              severity: 'error',
              message: 'condition "Country=us" is not supported',
            },
          ],
        }),
    })
    wrap(<StaticRoutesPanel app="docs" />)
    await screen.findByText('/old')
    fireEvent.change(screen.getByLabelText('_redirects'), {
      target: { value: '/a /b Country=us' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Check' }))
    await waitFor(() =>
      expect(
        screen.getByText(/0 rules and 0 header blocks parsed, 1 lines skipped/),
      ).toBeTruthy(),
    )
    expect(screen.getByText(/is not supported/)).toBeTruthy()
  })
})
