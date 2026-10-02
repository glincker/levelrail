import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateNetworkShareDialog } from './CreateNetworkShareDialog'

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

// Same "METHOD url -> handler table" shape
// CreateRegistryCredentialDialog.test.tsx's own mockFetchRoutes
// establishes, reused here rather than reinvented.
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

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <CreateNetworkShareDialog />
    </QueryClientProvider>,
  )
}

async function openDialog() {
  fireEvent.click(screen.getByText('Add network share'))
  await screen.findByText(/An NFS export or CIFS\/SMB share/)
}

function jsonRoute(body: unknown, status = 200): () => Promise<Response> {
  return () => Promise.resolve(fakeJsonResponse(body, status))
}

describe('CreateNetworkShareDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('defaults to nfs and hides username/password fields', async () => {
    mockFetchRoutes({})
    renderDialog()
    await openDialog()

    expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument()
  })

  it('submits an nfs share with no username or password', async () => {
    const fetchMock = mockFetchRoutes({
      'POST /api/v1/network-shares': jsonRoute({
        id: 'ns_1',
        name: 'media-nas',
        protocol: 'nfs',
        host: 'nas.lan',
        remote_path: '/export/media',
        created_at: '2026-10-01T00:00:00Z',
      }),
    })
    renderDialog()
    await openDialog()

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'media-nas' },
    })
    fireEvent.change(screen.getByLabelText('Host'), {
      target: { value: 'nas.lan' },
    })
    fireEvent.change(screen.getByLabelText('Export path'), {
      target: { value: '/export/media' },
    })

    const submitButton = document.querySelector('form button[type="submit"]')
    if (!submitButton) throw new Error('no submit button found')
    fireEvent.click(submitButton)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/network-shares',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    const call = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    )
    const body = JSON.parse((call?.[1] as RequestInit).body as string) as {
      protocol: string
      username?: string
      password?: string
    }
    expect(body.protocol).toBe('nfs')
    expect(body.username).toBeUndefined()
    expect(body.password).toBeUndefined()
  })
})
