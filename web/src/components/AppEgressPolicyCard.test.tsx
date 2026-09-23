import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppEgressPolicyCard } from './AppEgressPolicyCard'
import type { Brand } from '../types/brand'

// AppEgressPolicyCard now renders a HelpLink, which reads brand.DocsURL
// via useBrand; mocked the same way CreateRegistryCredentialDialog.test.tsx
// mocks it for its own brand.DocsURL-dependent rendering.
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
    DiscussionsURL: '',
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

function stubFetch({
  policy,
  conditions,
}: {
  policy: unknown
  conditions: unknown[]
}) {
  vi.stubGlobal(
    'fetch',
    vi.fn<(input: RequestInfo | URL, init?: RequestInit) => Promise<Response>>(
      (input) => {
        const url = requestUrlOf(input)
        if (url.endsWith('/egress-policy')) {
          return Promise.resolve(fakeJsonResponse(policy))
        }
        if (url.endsWith('/deploys')) {
          return Promise.resolve(fakeJsonResponse(conditions))
        }
        throw new Error(`unexpected fetch: ${url}`)
      },
    ),
  )
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={<div>loading</div>}>
        <AppEgressPolicyCard appName="demo-app" />
      </Suspense>
    </QueryClientProvider>,
  )
}

describe('AppEgressPolicyCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows unrestricted egress when no policy is configured', async () => {
    stubFetch({ policy: { app_name: 'demo-app' }, conditions: [] })

    renderCard()

    expect(await screen.findByText('Unrestricted egress')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Restrict outbound traffic' }),
    ).toBeInTheDocument()
  })

  it('shows the allowlist mode badge and rules once configured', async () => {
    stubFetch({
      policy: {
        app_name: 'demo-app',
        mode: 'allowlist',
        allow: [{ host: 'api.example.com', port: 443 }],
      },
      conditions: [
        {
          Type: 'EgressPolicyReady',
          Status: 'True',
          Reason: 'PoliciesApplied',
          Message: '',
          LastTransitionTime: '2026-01-01T00:00:00Z',
        },
      ],
    })

    renderCard()

    expect(await screen.findByText('Allowlist mode')).toBeInTheDocument()
    expect(screen.getByText('api.example.com:443')).toBeInTheDocument()
    expect(screen.getByText('Enforced')).toBeInTheDocument()
  })

  it('shows not-enforced status and the failure message when the sidecar failed to apply', async () => {
    stubFetch({
      policy: {
        app_name: 'demo-app',
        mode: 'allowlist',
        allow: [{ host: 'api.example.com', port: 443 }],
      },
      conditions: [
        {
          Type: 'EgressPolicyReady',
          Status: 'False',
          Reason: 'PolicyApplyFailed',
          Message: 'awaiting the app container to be running',
          LastTransitionTime: '2026-01-01T00:00:00Z',
        },
      ],
    })

    renderCard()

    expect(await screen.findByText('Not enforced')).toBeInTheDocument()
    expect(
      screen.getByText('awaiting the app container to be running'),
    ).toBeInTheDocument()
  })

  it('adds and removes rule rows while editing', async () => {
    stubFetch({ policy: { app_name: 'demo-app' }, conditions: [] })
    renderCard()

    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', { name: 'Restrict outbound traffic' }),
    )

    expect(screen.getAllByLabelText('Allowed host')).toHaveLength(1)

    await user.click(screen.getByRole('button', { name: 'Add rule' }))
    expect(screen.getAllByLabelText('Allowed host')).toHaveLength(2)

    await user.click(screen.getAllByRole('button', { name: 'Remove rule' })[1]!)
    expect(screen.getAllByLabelText('Allowed host')).toHaveLength(1)
  })

  it('PUTs mode/allow on save', async () => {
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      const url = requestUrlOf(input)
      if (url.endsWith('/egress-policy') && init?.method === 'PUT') {
        return Promise.resolve(
          fakeJsonResponse({
            app_name: 'demo-app',
            mode: 'allowlist',
            allow: [{ host: 'api.example.com', port: 443 }],
          }),
        )
      }
      if (url.endsWith('/egress-policy')) {
        return Promise.resolve(fakeJsonResponse({ app_name: 'demo-app' }))
      }
      if (url.endsWith('/deploys')) {
        return Promise.resolve(fakeJsonResponse([]))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    renderCard()

    const user = userEvent.setup()
    await user.click(
      await screen.findByRole('button', { name: 'Restrict outbound traffic' }),
    )
    await user.type(screen.getByLabelText('Allowed host'), 'api.example.com')
    await user.type(screen.getByLabelText('Allowed port'), '443')
    await user.click(screen.getByRole('button', { name: 'Save allowlist' }))

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'PUT',
      )
      expect(putCall).toBeDefined()
      const body = JSON.parse((putCall?.[1]?.body as string) ?? '{}') as {
        mode: string
        allow: { host: string; port: number }[]
      }
      expect(body.mode).toBe('allowlist')
      expect(body.allow).toEqual([{ host: 'api.example.com', port: 443 }])
    })
  })
})
