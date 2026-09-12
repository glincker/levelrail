import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateRegistryCredentialDialog } from './CreateRegistryCredentialDialog'
import type { Brand } from '../types/brand'

vi.mock('../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: 'https://test.example/docs',
  }),
}))

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

// Same "METHOD url -> handler table" shape RegistryImagePicker.test.tsx's
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

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <CreateRegistryCredentialDialog />
    </QueryClientProvider>,
  )
}

// Mirrors RegistryImagePicker.test.tsx's own pickOption: retries the
// open+click pair on base-ui's Select, since a same-tick click can be
// dropped under concurrent test-file load.
async function pickOption(
  triggerId: string,
  optionText: string,
  settled: () => void,
) {
  const trigger = document.querySelector(`#${triggerId}`)
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

async function openDialog() {
  fireEvent.click(screen.getByText('Add credential'))
  await screen.findByText(/Referenced by name from app\.yaml/)
}

describe('CreateRegistryCredentialDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    document.body.style.pointerEvents = ''
  })

  it('picking the Docker Hub preset fills registry_host with the fixed hostname, read-only', async () => {
    mockFetchRoutes({})
    renderDialog()
    await openDialog()

    const hostInput = document.querySelector(
      '#registry-credential-host',
    ) as HTMLInputElement

    await pickOption('registry-credential-host-preset', 'Docker Hub', () => {
      expect(hostInput.value).toBe('docker.io')
    })
    expect(hostInput).toHaveAttribute('readonly')
  })

  it('picking the GHCR preset fills registry_host with ghcr.io, read-only', async () => {
    mockFetchRoutes({})
    renderDialog()
    await openDialog()

    const hostInput = document.querySelector(
      '#registry-credential-host',
    ) as HTMLInputElement

    await pickOption(
      'registry-credential-host-preset',
      'GitHub Container Registry (GHCR)',
      () => {
        expect(hostInput.value).toBe('ghcr.io')
      },
    )
    expect(hostInput).toHaveAttribute('readonly')
  })

  it('picking an account-specific preset like ECR clears the field and drops to editable free text with a hint', async () => {
    mockFetchRoutes({})
    renderDialog()
    await openDialog()

    const hostInput = document.querySelector(
      '#registry-credential-host',
    ) as HTMLInputElement

    await pickOption(
      'registry-credential-host-preset',
      'AWS Elastic Container Registry (ECR)',
      () => {
        expect(hostInput.value).toBe('')
      },
    )
    expect(hostInput).not.toHaveAttribute('readonly')
    expect(hostInput.placeholder).toBe(
      '123456789012.dkr.ecr.us-east-1.amazonaws.com',
    )
    expect(
      screen.getByText(/Per-AWS-account hostname/),
    ).toBeInTheDocument()

    fireEvent.change(hostInput, {
      target: { value: '999999999999.dkr.ecr.eu-west-1.amazonaws.com' },
    })
    expect(hostInput.value).toBe('999999999999.dkr.ecr.eu-west-1.amazonaws.com')
  })

  it('Custom stays a plain free-text field and defaults to it on open', async () => {
    mockFetchRoutes({})
    renderDialog()
    await openDialog()

    const hostInput = document.querySelector(
      '#registry-credential-host',
    ) as HTMLInputElement
    expect(hostInput).not.toHaveAttribute('readonly')

    fireEvent.change(hostInput, { target: { value: 'registry.internal.example' } })
    expect(hostInput.value).toBe('registry.internal.example')
  })

  it('submits the exact preset hostname as registry_host, unchanged, on the create request', async () => {
    const fetchMock = mockFetchRoutes({
      'POST /api/v1/registry-credentials': jsonRoute({
        id: 'cred-1',
        name: 'ghcr-bot',
        registry_host: 'ghcr.io',
        username: 'bot',
        created_at: '2026-01-01T00:00:00Z',
      }),
    })
    renderDialog()
    await openDialog()

    await pickOption('registry-credential-host-preset', 'GitHub Container Registry (GHCR)', () => {
      const hostInput = document.querySelector(
        '#registry-credential-host',
      ) as HTMLInputElement
      expect(hostInput.value).toBe('ghcr.io')
    })

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'ghcr-bot' },
    })
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'bot' },
    })
    fireEvent.change(screen.getByLabelText(/Password/), {
      target: { value: 'super-secret' },
    })

    const submitButton = document.querySelector('form button[type="submit"]')
    if (!submitButton) throw new Error('no submit button found')
    fireEvent.click(submitButton)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/registry-credentials',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    const call = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    )
    const body = JSON.parse((call?.[1] as RequestInit).body as string) as {
      registry_host: string
    }
    expect(body.registry_host).toBe('ghcr.io')
  })
})

function jsonRoute(body: unknown, status = 200): () => Promise<Response> {
  return () => Promise.resolve(fakeJsonResponse(body, status))
}
