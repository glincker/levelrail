import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { UserEvent } from '@testing-library/user-event'
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

// Scoped to ui/select.tsx's own SelectItem markup (data-slot="select-item")
// rather than a plain screen.getByText: PromoteAppDialog also renders the
// preview panel's "Target: <app>" line with the same app name once a
// preview loads, so a text-only lookup is ambiguous the moment both are
// on screen at once.
function selectOptionByText(text: string): Element {
  const items = Array.from(
    document.body.querySelectorAll('[data-slot="select-item"]'),
  )
  const match = items.find((el) => el.textContent?.trim() === text)
  if (!match) throw new Error(`no select option with text "${text}"`)
  return match
}

function selectOptionTexts(): string[] {
  return Array.from(
    document.body.querySelectorAll('[data-slot="select-item"]'),
  ).map((el) => el.textContent?.trim() ?? '')
}

function triggerById(id: string): Element {
  const el = document.body.querySelector(`#${id}`)
  if (!el) throw new Error(`no trigger with id ${id}`)
  return el
}

// Opens via fireEvent (a plain synthetic click, unaffected by base-ui's
// pointer-events:none guard during popup positioning) then picks via
// userEvent (whose fuller hover/focus simulation is what actually gets
// base-ui's Select.Item to commit a selection in jsdom; a bare
// fireEvent.click on the item is a no-op there). Retries the whole
// open+pick pair since base-ui can occasionally drop the interaction
// under concurrent test-file load, the same shape GitRepoSourcePicker.
// test.tsx's own pickOption documents.
async function pickOption(
  user: UserEvent,
  triggerId: string,
  optionText: string,
) {
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(triggerById(triggerId))
    try {
      await user.click(selectOptionByText(optionText))
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

// The last matching call, not the first: react-query refires this query
// on every environmentId/target change, so an early "to=..." call with no
// target yet is still sitting in fetchMock.mock.calls once a target is
// picked afterward.
function findPreviewCallUrl(fetchMock: ReturnType<typeof vi.fn>): string | undefined {
  const calls = fetchMock.mock.calls.filter(([input]) =>
    requestUrlOf(input as RequestInfo | URL).includes('/promote/preview'),
  )
  const last = calls.at(-1)
  return last ? requestUrlOf(last[0] as RequestInfo | URL) : undefined
}

describe('PromoteAppDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('offers only apps tagged with the picked environment in this project, excludes the app being promoted, plus auto-detect', async () => {
    mockFetchRoutes({
      'GET /api/v1/projects/proj1/environments': jsonRoute(environments),
      'GET /api/v1/apps': jsonRoute(apps),
      'GET /api/v1/apps/web/promote/preview': jsonRoute(fakePreview('web-staging')),
    })
    const user = userEvent.setup()
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption(user, 'promote-target-environment', 'staging')

    fireEvent.click(triggerById('promote-target-app'))
    const options = selectOptionTexts()
    expect(options).toContain('web-staging')
    expect(options).not.toContain('web-prod')
    expect(options).not.toContain('other-staging')
    // The app being promoted never lists itself as a target.
    expect(options).not.toContain('web')
  })

  it('sends no target param when auto-detect stays selected', async () => {
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/projects/proj1/environments': jsonRoute(environments),
      'GET /api/v1/apps': jsonRoute(apps),
      'GET /api/v1/apps/web/promote/preview': jsonRoute(fakePreview('web-staging')),
    })
    const user = userEvent.setup()
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption(user, 'promote-target-environment', 'staging')

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
    const user = userEvent.setup()
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption(user, 'promote-target-environment', 'staging')
    await pickOption(user, 'promote-target-app', 'web-staging')

    await waitFor(() => {
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
    const user = userEvent.setup()
    renderDialog()
    fireEvent.click(screen.getByText('Promote to...'))
    await screen.findByLabelText('Environment')

    await pickOption(user, 'promote-target-environment', 'staging')

    expect(
      await screen.findByText(/No other apps in this project are tagged/),
    ).toBeInTheDocument()
  })
})
