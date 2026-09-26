import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AlertHistoryTable } from './AlertHistoryTable'
import { AlertSilencesPanel } from './AlertSilencesPanel'
import { SilenceRuleMenu } from './SilenceRuleMenu'
import { StatusComponentsPanel } from './StatusComponentsPanel'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderWithClient(node: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(<QueryClientProvider client={queryClient}>{node}</QueryClientProvider>)
}

describe('alert noise UI', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('quick silence posts the chosen duration for the rule', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >(() => Promise.resolve(json({ id: 'sil_1' }, 201)))
    vi.stubGlobal('fetch', fetchMock)

    renderWithClient(
      <SilenceRuleMenu appName="web" ruleId="r1" ruleName="high cpu" />,
    )
    await user.click(screen.getByRole('button', { name: /silence high cpu/i }))
    await user.click(await screen.findByText('Silence for 4h'))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(urlOf(url as RequestInfo)).toBe('/api/v1/apps/web/alerts/r1/silence')
    expect(init?.method).toBe('POST')
    expect(JSON.parse(init?.body as string)).toEqual({ duration: '4h' })
  })

  it('lists silences and ends one now', async () => {
    const user = userEvent.setup()
    const silence = {
      id: 'sil_9',
      matchers: { apps: ['web'] },
      starts_at: '2026-09-25T10:00:00Z',
      ends_at: '2026-09-25T14:00:00Z',
      created_by: 'alice',
      reason: 'migration',
      created_at: '2026-09-25T10:00:00Z',
      status: 'active',
    }
    const fetchMock = vi.fn<
      (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
    >((input, init) => {
      if (init?.method === 'DELETE') {
        return Promise.resolve(json({ ...silence, status: 'expired' }))
      }
      expect(urlOf(input)).toContain('/api/v1/alert-silences')
      return Promise.resolve(json([silence]))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithClient(<AlertSilencesPanel />)
    await screen.findByText('app: web')
    expect(screen.getByText('migration')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /end now/i }))
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(
          ([input, init]) =>
            init?.method === 'DELETE' &&
            urlOf(input) === '/api/v1/alert-silences/sil_9',
        ),
      ).toBe(true)
    })
  })

  it('shows history rows with their notification outcome', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          json([
            {
              id: 'ah_1',
              at: '2026-09-25T10:00:00Z',
              rule_id: 'r1',
              rule_name: 'high cpu',
              rule_kind: 'threshold',
              app: 'web',
              event: 'fired',
              outcome: 'silenced',
              detail: 'silence sil_9',
            },
          ]),
        ),
      ),
    )
    renderWithClient(<AlertHistoryTable app="web" />)
    await screen.findByText('high cpu')
    expect(screen.getByText('silenced')).toBeInTheDocument()
    expect(screen.getByText('silence sil_9')).toBeInTheDocument()
  })

  it('status components form explains that only the public name is shown', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(json([]))),
    )
    renderWithClient(<StatusComponentsPanel />)
    expect(
      await screen.findByText(/only the public name is ever shown/i),
    ).toBeInTheDocument()
    expect(screen.getByLabelText(/public name/i)).toBeInTheDocument()
  })
})
