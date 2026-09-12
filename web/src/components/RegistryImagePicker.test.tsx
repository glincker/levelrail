import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RegistryImagePicker } from './RegistryImagePicker'
import type { RegistrySettings } from '../queries/registry'
import type { RegistryCredential } from '../types/registryCredential'

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

// Same "METHOD url -> handler table" shape GitRepoSourcePicker.test.tsx's
// own mockFetchRoutes establishes, reused here rather than reinvented.
function mockFetchRoutes(
  routes: Record<string, () => Promise<Response>>,
): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = requestUrlOf(input)
    const method = init?.method ?? 'GET'
    const handler = routes[`${method} ${url}`]
    if (handler) return handler()
    return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function jsonRoute(body: unknown, status = 200): () => Promise<Response> {
  return () => Promise.resolve(fakeJsonResponse(body, status))
}

const runningRegistry: RegistrySettings = {
  enabled: true,
  host: 'registry.example',
  username: 'levelrail',
  has_credentials: true,
  status: 'running',
}

const noCredentials: () => Promise<Response> = jsonRoute([])

const oneCredential: RegistryCredential = {
  id: 'cred-1',
  name: 'ghcr',
  registry_host: 'ghcr.io',
  username: 'gdsks',
  created_at: '2026-01-01T00:00:00Z',
}

function renderPicker() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const onSelect = vi.fn()
  const result = render(
    <QueryClientProvider client={queryClient}>
      <RegistryImagePicker onSelect={onSelect} />
    </QueryClientProvider>,
  )
  return { onSelect, container: result.container }
}

// Mirrors GitRepoSourcePicker.test.tsx's own pickOption: retries the
// open+click pair on base-ui's Select, since a same-tick click can be
// dropped under concurrent test-file load.
async function pickOption(
  container: HTMLElement,
  triggerId: string,
  optionText: string,
  settled: () => void,
) {
  const trigger = container.querySelector(`#${triggerId}`)
  if (!trigger) throw new Error(`no trigger with id ${triggerId}`)
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

describe('RegistryImagePicker', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.style.pointerEvents = ''
  })

  it('renders nothing when the built-in registry is disabled', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute({ enabled: false, status: 'stopped', has_credentials: false }),
      'GET /api/v1/registry-credentials': noCredentials,
    })

    const { container } = renderPicker()

    await waitFor(() => {
      expect(container.querySelector('#registry-picker-repo')).not.toBeInTheDocument()
    })
  })

  it('renders nothing when enabled but not yet running', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute({
        enabled: true,
        host: 'registry.example',
        status: 'stopped',
        has_credentials: true,
      }),
      'GET /api/v1/registry-credentials': noCredentials,
    })

    const { container } = renderPicker()

    await waitFor(() => {
      expect(container.querySelector('#registry-picker-repo')).not.toBeInTheDocument()
    })
  })

  it('shows the repository picker once the registry is running, and an empty-state note when there are none', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute(runningRegistry),
      'GET /api/v1/registry-credentials': noCredentials,
      'GET /api/v1/registry/repositories': jsonRoute({ repositories: [] }),
    })

    renderPicker()

    expect(await screen.findByText('Repository')).toBeInTheDocument()
    expect(
      await screen.findByText('No images have been pushed to the built-in registry yet.'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Tag')).not.toBeInTheDocument()
    expect(screen.queryByText('Registry')).not.toBeInTheDocument()
  })

  it('picking a repository then a tag calls onSelect with the full host/repository:tag reference', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute(runningRegistry),
      'GET /api/v1/registry-credentials': noCredentials,
      'GET /api/v1/registry/repositories': jsonRoute({ repositories: ['myapp'] }),
      'GET /api/v1/registry/tags?repository=myapp': jsonRoute({ repository: 'myapp', tags: ['latest', 'v1'] }),
    })

    const { onSelect, container } = renderPicker()

    await screen.findByText('Repository')
    await pickOption(container, 'registry-picker-repo', 'myapp', () => {
      expect(container.querySelector('#registry-picker-tag')).toBeInTheDocument()
    })

    await screen.findByText('Tag')
    await pickOption(container, 'registry-picker-tag', 'latest', () => {
      expect(onSelect).toHaveBeenCalledWith('registry.example/myapp:latest')
    })
  })

  it('surfaces a repository list fetch error inline', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute(runningRegistry),
      'GET /api/v1/registry-credentials': noCredentials,
      'GET /api/v1/registry/repositories': jsonRoute({ error: 'internal error' }, 500),
    })

    renderPicker()

    expect(await screen.findByText(/internal error/i)).toBeInTheDocument()
  })

  it('shows a registry source selector once a credential is connected, defaulting to the built-in registry', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute(runningRegistry),
      'GET /api/v1/registry-credentials': jsonRoute([oneCredential]),
      'GET /api/v1/registry/repositories': jsonRoute({ repositories: ['myapp'] }),
    })

    const { container } = renderPicker()

    expect(await screen.findByText('Registry')).toBeInTheDocument()

    await pickOption(container, 'registry-picker-repo', 'myapp', () => {
      expect(container.querySelector('#registry-picker-tag')).toBeInTheDocument()
    })
  })

  it('browsing a connected credential queries its own catalog and calls onSelect with its host', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute({ enabled: false, status: 'stopped', has_credentials: false }),
      'GET /api/v1/registry-credentials': jsonRoute([oneCredential]),
      'GET /api/v1/registry-credentials/cred-1/repositories': jsonRoute({ repositories: ['org/app'] }),
      'GET /api/v1/registry-credentials/cred-1/tags?repository=org%2Fapp': jsonRoute({
        repository: 'org/app',
        tags: ['v2'],
      }),
    })

    const { onSelect, container } = renderPicker()

    await screen.findByText('Registry')
    await pickOption(container, 'registry-picker-repo', 'org/app', () => {
      expect(container.querySelector('#registry-picker-tag')).toBeInTheDocument()
    })

    await screen.findByText('Tag')
    await pickOption(container, 'registry-picker-tag', 'v2', () => {
      expect(onSelect).toHaveBeenCalledWith('ghcr.io/org/app:v2')
    })
  })

  it('shows an empty-state note and surfaces fetch errors for a connected credential with no repositories', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute({ enabled: false, status: 'stopped', has_credentials: false }),
      'GET /api/v1/registry-credentials': jsonRoute([oneCredential]),
      'GET /api/v1/registry-credentials/cred-1/repositories': jsonRoute({ repositories: [] }),
    })

    renderPicker()

    expect(
      await screen.findByText('No repositories found in this registry yet.'),
    ).toBeInTheDocument()
  })
})
