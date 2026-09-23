import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BranchEnvOverridesEditor } from './BranchEnvOverridesEditor'

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

function renderEditor() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <BranchEnvOverridesEditor appName="demo-app" />
    </QueryClientProvider>,
  )
}

describe('BranchEnvOverridesEditor', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('lists existing overrides, hiding a secret one’s value', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse([
            {
              id: 'benv_1',
              branch_pattern: 'release/*',
              key: 'FEATURE_FLAG',
              value: 'on',
              secret: false,
              updated_at: '2026-01-01T00:00:00Z',
            },
            {
              id: 'benv_2',
              branch_pattern: 'main',
              key: 'API_KEY',
              value: '',
              secret: true,
              updated_at: '2026-01-01T00:00:00Z',
            },
          ]),
        ),
      ),
    )

    renderEditor()

    expect(await screen.findByText('FEATURE_FLAG')).toBeInTheDocument()
    expect(screen.getAllByText('release/*').length).toBeGreaterThan(0)
    expect(screen.getByText('on')).toBeInTheDocument()
    expect(screen.getByText('API_KEY')).toBeInTheDocument()
    expect(screen.getByText('hidden')).toBeInTheDocument()
    expect(screen.queryByText('sk-')).not.toBeInTheDocument()
  })

  it('POSTs branch_pattern/key/value/secret on save', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      const url = requestUrlOf(input)
      if (!url.endsWith('/branch-env')) {
        throw new Error(`unexpected fetch: ${url}`)
      }
      if (init?.method === 'POST') {
        return Promise.resolve(
          fakeJsonResponse({
            id: 'benv_1',
            branch_pattern: 'release/*',
            key: 'FEATURE_FLAG',
            value: 'on',
            secret: false,
            updated_at: '2026-01-01T00:00:00Z',
          }),
        )
      }
      return Promise.resolve(fakeJsonResponse([]))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderEditor()

    const user = userEvent.setup()
    await user.type(
      await screen.findByLabelText('Branch or pattern'),
      'release/*',
    )
    await user.type(screen.getByLabelText('Env var'), 'FEATURE_FLAG')
    await user.type(screen.getByLabelText('Value'), 'on')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'POST',
      )
      expect(postCall).toBeDefined()
      const body = JSON.parse((postCall?.[1]?.body as string) ?? '{}') as {
        branch_pattern: string
        key: string
        value: string
        secret: boolean
      }
      expect(body.branch_pattern).toBe('release/*')
      expect(body.key).toBe('FEATURE_FLAG')
      expect(body.value).toBe('on')
      expect(body.secret).toBe(false)
    })
  })

  it('sends secret=true and masks the value input when the checkbox is checked', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((_input, init) => {
      if (init?.method === 'POST') {
        return Promise.resolve(
          fakeJsonResponse({
            id: 'benv_3',
            branch_pattern: 'main',
            key: 'API_KEY',
            value: '',
            secret: true,
            updated_at: '2026-01-01T00:00:00Z',
          }),
        )
      }
      return Promise.resolve(fakeJsonResponse([]))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderEditor()

    const user = userEvent.setup()
    await user.type(await screen.findByLabelText('Branch or pattern'), 'main')
    await user.type(screen.getByLabelText('Env var'), 'API_KEY')
    await user.click(screen.getByRole('checkbox', { name: /store encrypted/i }))
    const valueInput = screen.getByLabelText('Value')
    expect(valueInput).toHaveAttribute('type', 'password')
    await user.type(valueInput, 'sk-live-real')
    await user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'POST',
      )
      expect(postCall).toBeDefined()
      const body = JSON.parse((postCall?.[1]?.body as string) ?? '{}') as {
        secret: boolean
        value: string
      }
      expect(body.secret).toBe(true)
      expect(body.value).toBe('sk-live-real')
    })
  })

  it('DELETEs the override by id when Remove is clicked', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      const url = requestUrlOf(input)
      if (init?.method === 'DELETE') {
        expect(url).toContain('/branch-env/benv_1')
        return Promise.resolve({ ok: true, status: 204 } as Response)
      }
      return Promise.resolve(
        fakeJsonResponse([
          {
            id: 'benv_1',
            branch_pattern: 'release/*',
            key: 'FEATURE_FLAG',
            value: 'on',
            secret: false,
            updated_at: '2026-01-01T00:00:00Z',
          },
        ]),
      )
    })
    vi.stubGlobal('fetch', fetchMock)

    renderEditor()

    const removeButton = await screen.findByRole('button', {
      name: /remove feature_flag for release\/\*/i,
    })
    const user = userEvent.setup()
    await user.click(removeButton)

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([, init]) => init?.method === 'DELETE'),
      ).toBe(true)
    })
  })
})
