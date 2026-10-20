import { render, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  type AnyRoute,
  type RouteComponent,
} from '@tanstack/react-router'
import { Suspense, type ComponentType } from 'react'
import { expect, vi } from 'vitest'
import { BrandProvider } from '@/components/BrandProvider'
import { ThemeProvider } from '@/components/ThemeProvider'
import { AllI18n } from './allI18n'
import { a11yViolations } from './a11y'

const BRAND = {
  Name: 'Brand',
  ShortName: 'brand',
  BinaryName: 'brand',
  Domain: 'example.test',
  SupportURL: 'https://example.test/support',
  PrimaryColor: '#107292',
  LogoSVG: '',
  DocsURL: 'https://docs.example.test',
  DiscussionsURL: 'https://example.test/d',
  RepoURL: 'https://example.test/r',
}

const SESSION = {
  username: 'admin@example.test',
  display_name: 'Admin',
  role: 'root',
}

type Fixture = [RegExp, unknown]

function stubApi(extra: Fixture[]) {
  const fixtures: Fixture[] = [
    ...extra,
    [/\/api\/v1\/brand$/, BRAND],
    [/\/api\/v1\/dev-mode$/, { experimental: ['plugins'] }],
    [
      /\/api\/v1\/apps\/web$/,
      {
        name: 'web',
        image: 'nginx:1.27',
        port: 80,
        bind_address: '',
        strategy: 'rolling',
        replicas: 1,
        domains: [],
      },
    ],
    [/\/log-budget$/, { max_mb: 0, default_mb: 512 }],
    [/\/api\/v1\/logs\/usage$/, { apps: [] }],
    [
      /\/api\/v1\/databases\/pg$/,
      { name: 'pg', engine: 'postgres', version: '16', status: {} },
    ],
    [
      /\/api\/v1\/security\/posture$/,
      {
        score: 90,
        grade: 'A',
        generated_at: '2026-10-19T00:00:00Z',
        counts: {
          critical: 0,
          high: 0,
          medium: 0,
          low: 0,
          unknown: 0,
          passing: 0,
        },
        full: true,
        items: [],
        account: [],
      },
    ],
    [/\/api\/v1\/demo$/, { enabled: false }],
    [/\/api\/v1\/(auth\/)?(session|me)$/, SESSION],
    [/\/api\/v1\/preferences$/, {}],
  ]
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: false,
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    onchange: null,
    dispatchEvent: () => false,
  }))
  vi.stubGlobal(
    'EventSource',
    class {
      close() {}
      addEventListener() {}
      removeEventListener() {}
      onmessage = null
      onerror = null
    },
  )
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : (input as URL).toString()
      const hit = fixtures.find(([re]) => re.test(url.split('?')[0] ?? ''))
      const body = hit ? hit[1] : url.includes('?') || /s$/.test(url) ? [] : {}
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

export interface PageOptions {
  /** Route path pattern, e.g. /apps/$name/overview. Defaults to /. */
  path?: string
  /** Concrete URL to visit. Defaults to the path. */
  entry?: string
  fixtures?: Fixture[]
}

/** Mounts one real route (or a bare component) and returns a11y violations. */
export async function pageViolations(
  target: AnyRoute | ComponentType,
  ready: (root: HTMLElement) => boolean,
  opts: PageOptions = {},
): Promise<string[]> {
  stubApi(opts.fixtures ?? [])
  window.localStorage.setItem('app-auth-username', SESSION.username)
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const root = createRootRoute({ component: Outlet })
  const path = opts.path ?? '/'
  const route =
    typeof target === 'function'
      ? createRoute({
          getParentRoute: () => root,
          path,
          component: target as RouteComponent,
        })
      : target.update({
          id: path,
          path,
          getParentRoute: () => root,
        } as never)
  const router = createRouter({
    routeTree: root.addChildren([route]),
    history: createMemoryHistory({ initialEntries: [opts.entry ?? path] }),
    context: { queryClient },
  })
  render(
    <AllI18n>
      <QueryClientProvider client={queryClient}>
        <ThemeProvider>
          <BrandProvider>
            <Suspense fallback={null}>
              <RouterProvider router={router} />
            </Suspense>
          </BrandProvider>
        </ThemeProvider>
      </QueryClientProvider>
    </AllI18n>,
  )
  try {
    await waitFor(() => expect(ready(document.body)).toBe(true), {
      timeout: 8000,
    })
  } catch {
    const text = document.body.textContent?.slice(0, 300) ?? ''
    throw new Error(`page never became ready: ${text}`)
  }
  await new Promise((r) => setTimeout(r, 200))
  return a11yViolations(document.body)
}
