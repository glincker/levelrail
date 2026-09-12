import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PromoteAppDialog } from './PromoteAppDialog'
import type { AppListEntry } from '../types/appDetail'
import type { EnvironmentResource } from '../types/environment'
import type { PromotePreviewResource } from '../types/promote'

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function fakeJsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

// Same route-table stub GitRepoSourcePicker.test.tsx already uses: each
// test states which endpoints it needs and what they return, keyed on
// path only (query strings are stripped since promote/preview varies its
// `target` param across assertions and the fetch calls are inspected
// separately via fetchMock.mock.calls).
function mockFetchRoutes(
  routes: Record<string, () => Promise<Response>>,
): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = requestUrlOf(input)
    const method = init?.method ?? 'GET'
    const path = url.split('?')[0]
    const handler = routes[`${method} ${path}`]
    if (handler) return handler()
    return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function jsonRoute(body: unknown, status = 200): () => Promise<Response> {
  return () => Promise.resolve(fakeJsonResponse(body, status))
}

// Dialog content (and every Select trigger inside it) renders through a
// base-ui portal appended to document.body, a sibling of RTL's own render
// container rather than a descendant of it, so lookups below search the
// whole body instead of a local container.
function triggerById(id: string): Element {
  const el = document.body.querySelector(`#${id}`)
  if (!el) throw new Error(`no trigger with id ${id}`)
  return el
}

// Identical retry shape to GitRepoSourcePicker.test.tsx's own pickOption:
// base-ui's Select occasionally drops a synthetic click sent in the same
// tick a popup opens, observed as the popup staying open and the option
// click never landing. A handful of real-clock-spaced attempts converges
// on the same "keep trying until it lands" behavior without turning into
// a busy loop under waitFor's own immediate-retry semantics.
async function pickOption(
  triggerId: string,
  optionText: string,
  settled: () => void,
) {
  const trigger = triggerById(triggerId)
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(trigger)
    try {
      fireEvent.click(screen.getByText(optionText))
      settled()
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

// Same shape as pickOption, but only opens the popup and asserts what's
// inside without clicking anything: used to check the candidate list
// itself (which apps got filtered in/out), not a selection.
async function openAndInspect(triggerId: string, inspect: () => void) {
  const trigger = triggerById(triggerId)
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(trigger)
    try {
      inspect()
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

const stagingEnv: EnvironmentResource = {
  id: 'env-staging',
  project_id: 'proj1',
  name: 'staging',
  protected: false,
  created_at: '2026-01-01T00:00:00Z',
}

const prodEnv: EnvironmentResource = {
  id: 'env-prod',
  project_id: 'proj1',
  name: 'prod',
  protected: false,
  created_at: '2026-01-01T00:00:00Z',
}

const environments: EnvironmentResource[] = [stagingEnv, prodEnv]

function fakeApp(overrides: Partial<AppListEntry>): AppListEntry {
  return {
    name: 'app',
    image: 'ghcr.io/acme/app:latest',
    port: 3000,
    strategy: 'rolling',
    replicas: 1,
    suspended: false,
    env_dirty: false,
    status: { label: 'Running', variant: 'success' },
    ...overrides,
  }
}

const apps: AppListEntry[] = [
  fakeApp({ name: 'web', project_id: 'proj1', environment_id: 'env-dev' }),
  fakeApp({ name: 'web-staging', project_id: 'proj1', environment_id: 'env-staging' }),
  fakeApp({ name: 'web-prod', project_id: 'proj1', environment_id: 'env-prod' }),
  // Different project: never a candidate, even if tagged staging.
  fakeApp({ name: 'other-staging', project_id: 'proj2', environment_id: 'env-staging' }),
]

function fakePreview(targetApp: string): PromotePreviewResource {
  return {
    source_app: 'web',
    target_app: targetApp,
    environment: stagingEnv,
    from: { app_name: 'web', image: 'ghcr.io/acme/app:v2' },
    to: { app_name: targetApp, image: 'ghcr.io/acme/app:v1' },
    changes: [{ field: 'image', from: 'ghcr.io/acme/app:v1', to: 'ghcr.io/acme/app:v2' }],
    unsnapshotted_fields: [],
    note: '',
  }
}

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <PromoteAppDialog appName="web" projectId="proj1" />
    </QueryClientProvider>,
  )
}

function findPreviewCallUrl(fetchMock: ReturnType<typeof vi.fn>): string | undefined {
  const call = fetchMock.mock.calls.find(([input]) =>
    requestUrlOf(input as RequestInfo | URL).includes('/promote/preview'),
  )
  return call ? requestUrlOf(call[0] as RequestInfo | URL) : undefined
}

describe('PromoteAppDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    // Same base-ui popup cleanup GitRepoSourcePicker.test.tsx applies: a
    // test ending right after opening a popup can outrun the async clear
    // of `pointer-events: none` on <body>, leaking into the next test.
    document.body.style.pointerEvents = ''
  })

  it('offers only apps tagged with the picked environment in this project, excludes the app being promoted, plus auto-detect', async () => {
    mockFetchRoutes({
      'GET /api/v1/projects/proj1/environments': jsonRoute(environments),
      'GET /api/v1/apps': jsonRoute(apps),
      'GET /api/v1/apps/web/promote/preview': jsonRoute(fakePreview('web-staging')),
    })
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption('promote-target-environment', 'staging', () => {
      expect(triggerById('promote-target-environment')).toHaveTextContent('staging')
    })

    await openAndInspect('promote-target-app', () => {
      expect(screen.getByText('web-staging')).toBeInTheDocument()
      expect(screen.queryByText('web-prod')).not.toBeInTheDocument()
      expect(screen.queryByText('other-staging')).not.toBeInTheDocument()
    })
  })

  it('sends no target param when auto-detect stays selected', async () => {
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/projects/proj1/environments': jsonRoute(environments),
      'GET /api/v1/apps': jsonRoute(apps),
      'GET /api/v1/apps/web/promote/preview': jsonRoute(fakePreview('web-staging')),
    })
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption('promote-target-environment', 'staging', () => {
      expect(triggerById('promote-target-environment')).toHaveTextContent('staging')
    })

    await waitFor(() => {
      const url = findPreviewCallUrl(fetchMock)
      expect(url).toBeTruthy()
      expect(url).not.toContain('target=')
    })
  })

  it('sends the picked app as the target param once selected', async () => {
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/projects/proj1/environments': jsonRoute(environments),
      'GET /api/v1/apps': jsonRoute(apps),
      'GET /api/v1/apps/web/promote/preview': jsonRoute(fakePreview('web-staging')),
    })
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption('promote-target-environment', 'staging', () => {
      expect(triggerById('promote-target-environment')).toHaveTextContent('staging')
    })

    await pickOption('promote-target-app', 'web-staging', () => {
      expect(triggerById('promote-target-app')).toHaveTextContent('web-staging')
    })

    await waitFor(() => {
      const url = findPreviewCallUrl(fetchMock)
      expect(url).toBeTruthy()
      expect(url).toContain('target=web-staging')
    })
  })

  it('shows a hint instead of an empty picker when no sibling app is tagged with the environment yet', async () => {
    mockFetchRoutes({
      'GET /api/v1/projects/proj1/environments': jsonRoute(environments),
      'GET /api/v1/apps': jsonRoute([
        fakeApp({ name: 'web', project_id: 'proj1', environment_id: 'env-dev' }),
      ]),
      'GET /api/v1/apps/web/promote/preview': jsonRoute(fakePreview('web')),
    })
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption('promote-target-environment', 'staging', () => {
      expect(
        screen.getByText(/No other apps in this project are tagged/),
      ).toBeInTheDocument()
    })
  })
})
