import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RegistryImagePicker } from './RegistryImagePicker'
import type { RegistrySettings } from '../queries/registry'

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
    })

    const { container } = renderPicker()

    await waitFor(() => {
      expect(container.querySelector('#registry-picker-repo')).not.toBeInTheDocument()
    })
  })

  it('shows the repository picker once the registry is running, and an empty-state note when there are none', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute(runningRegistry),
      'GET /api/v1/registry/repositories': jsonRoute({ repositories: [] }),
    })

    renderPicker()

    expect(await screen.findByText('Repository')).toBeInTheDocument()
    expect(
      await screen.findByText('No images have been pushed to the built-in registry yet.'),
    ).toBeInTheDocument()
    expect(screen.queryByText('Tag')).not.toBeInTheDocument()
  })

  it('picking a repository then a tag calls onSelect with the full host/repository:tag reference', async () => {
    mockFetchRoutes({
      'GET /api/v1/settings/registry': jsonRoute(runningRegistry),
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
      'GET /api/v1/registry/repositories': jsonRoute({ error: 'internal error' }, 500),
    })

    renderPicker()

    expect(await screen.findByText(/internal error/i)).toBeInTheDocument()
  })
})
