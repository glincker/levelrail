import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AlertingQuickSetupPrompt } from './AlertingQuickSetupPrompt'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: { children?: ReactNode; to?: string } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

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

function renderPrompt() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <AlertingQuickSetupPrompt carrierAppName="demo-app" />
    </QueryClientProvider>,
  )
}

describe('AlertingQuickSetupPrompt', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('offers to connect a channel first when none exist', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse([]))),
    )
    renderPrompt()

    await screen.findByText(/connect a notification channel first/i)
    expect(
      screen.getByRole('button', { name: /connect channel/i }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /enable recommended alerts/i }),
    ).not.toBeInTheDocument()
  })

  it('creates all four platform-wide rules against the carrier app and then hides itself', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<(input: RequestInfo | URL) => Promise<Response>>(
      (input) => {
        const url = requestUrlOf(input)
        if (url === '/api/v1/notification-channels') {
          return Promise.resolve(
            fakeJsonResponse([
              {
                id: 'chan_1',
                name: 'Team Slack',
                kind: 'slack',
                notify_url: 'https://hooks.slack.com/services/x',
                enabled: true,
                created_at: '2026-01-01T00:00:00.000000000Z',
                updated_at: '2026-01-01T00:00:00.000000000Z',
              },
            ]),
          )
        }
        if (url === '/api/v1/apps/demo-app/alerts') {
          return Promise.resolve(
            fakeJsonResponse({
              id: 'rule_1',
              name: 'rule',
              kind: 'cert_expiry',
              resource_id: 'app:demo-app',
              threshold: 0,
              restart_count_threshold: 0,
              enabled: true,
            }, 201),
          )
        }
        throw new Error(`unexpected fetch: ${url}`)
      },
    )
    vi.stubGlobal('fetch', fetchMock)

    renderPrompt()

    const enableButton = await screen.findByRole('button', {
      name: /enable recommended alerts/i,
    })
    await user.click(enableButton)

    await waitFor(() => {
      expect(
        screen.queryByText(/turn on platform-wide alerting/i),
      ).not.toBeInTheDocument()
    })

    const ruleCreateCalls = fetchMock.mock.calls.filter(
      (call) => requestUrlOf(call[0]) === '/api/v1/apps/demo-app/alerts',
    )
    expect(ruleCreateCalls).toHaveLength(4)
    const kinds = ruleCreateCalls.map((call) => {
      const init = call[1] as RequestInit
      return (JSON.parse(init.body as string) as { kind: string }).kind
    })
    expect(kinds).toEqual([
      'cert_expiry',
      'patch_status',
      'node_disk_space',
      'node_resource_usage',
    ])

    expect(window.localStorage.getItem('dashboard-alerting-quick-setup-dismissed')).toBe(
      '1',
    )
  })

  it('dismisses and does not reappear on remount', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse([]))),
    )
    const user = userEvent.setup()
    renderPrompt()

    const dismissButton = await screen.findByRole('button', {
      name: /dismiss for now/i,
    })
    await user.click(dismissButton)

    expect(
      screen.queryByText(/turn on platform-wide alerting/i),
    ).not.toBeInTheDocument()
    expect(
      window.localStorage.getItem('dashboard-alerting-quick-setup-dismissed'),
    ).toBe('1')

    renderPrompt()
    expect(
      screen.queryByText(/turn on platform-wide alerting/i),
    ).not.toBeInTheDocument()
  })
})
