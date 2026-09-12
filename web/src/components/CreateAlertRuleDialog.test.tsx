import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateAlertRuleDialog } from './CreateAlertRuleDialog'

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
// own mockFetchRoutes establishes.
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

// Mirrors RegistryImagePicker.test.tsx's own pickOption (retrying the
// open+click pair on base-ui's Select) plus a leading pointerDown:
// SelectItem only commits a plain click as a real selection once a
// pointerdown on that same item preceded it, which a real click always
// does but fireEvent.click alone does not synthesize.
async function pickOption(
  container: HTMLElement,
  triggerId: string,
  optionText: string,
) {
  const trigger = container.querySelector(`#${triggerId}`)
  if (!trigger) throw new Error(`no trigger with id ${triggerId}`)
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(trigger)
    try {
      const option = screen.getByText(optionText)
      fireEvent.pointerDown(option, { pointerType: 'mouse' })
      fireEvent.click(option)
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <CreateAlertRuleDialog appName="demo-app" />
    </QueryClientProvider>,
  )
}

// The dialog popup renders through a portal, outside RTL's own
// `container` root, so every subsequent lookup below is scoped to the
// popup element this returns (or to `screen`/`document`), never to
// `container`.
async function openDialog() {
  fireEvent.click(screen.getByRole('button', { name: 'Create rule' }))
  await screen.findByText('Create alert rule')
  const popup = document.querySelector('[data-slot="dialog-content"]')
  if (!popup) throw new Error('dialog content did not render')
  return popup as HTMLElement
}

describe('CreateAlertRuleDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders the metric field as a closed enum select, not free text', async () => {
    mockFetchRoutes({
      'GET /api/v1/notification-channels': jsonRoute([]),
      'GET /api/v1/apps/demo-app/scheduled-tasks': jsonRoute([]),
    })
    renderDialog()
    const popup = await openDialog()

    expect(popup.querySelector('input#rule-metric')).not.toBeInTheDocument()
    const trigger = popup.querySelector('#rule-metric')
    expect(trigger).toBeInTheDocument()
    expect(trigger?.tagName).toBe('BUTTON')

    fireEvent.click(trigger!)
    expect(await screen.findByText('CPU usage (%)')).toBeInTheDocument()
    expect(screen.getByText('Memory usage')).toBeInTheDocument()
    expect(screen.getByText('Disk write')).toBeInTheDocument()
  })

  it('renders the for-duration field as a number+unit DurationInput defaulting to 2 minutes', async () => {
    mockFetchRoutes({
      'GET /api/v1/notification-channels': jsonRoute([]),
      'GET /api/v1/apps/demo-app/scheduled-tasks': jsonRoute([]),
    })
    renderDialog()
    const popup = await openDialog()

    expect(popup.querySelector('input#rule-for-duration')).toBeInTheDocument()
    expect(
      (popup.querySelector('input#rule-for-duration') as HTMLInputElement)
        .value,
    ).toBe('2')
    expect(screen.getByText('Minutes')).toBeInTheDocument()
  })

  it('submits the chosen metric and a composed for_duration string', async () => {
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/notification-channels': jsonRoute([]),
      'GET /api/v1/apps/demo-app/scheduled-tasks': jsonRoute([]),
      'POST /api/v1/apps/demo-app/alerts': jsonRoute(
        {
          id: 'rule_1',
          name: 'high-mem',
          kind: 'threshold',
          resource_id: 'app:demo-app',
          metric: 'memory_usage_bytes',
          comparator: '>',
          threshold: 80,
          for_duration: '10m',
          restart_count_threshold: 0,
          enabled: true,
        },
        201,
      ),
    })
    renderDialog()
    const popup = await openDialog()

    fireEvent.change(within(popup).getByLabelText('Name'), {
      target: { value: 'high-mem' },
    })
    await pickOption(popup, 'rule-metric', 'Memory usage')

    const durationAmount = popup.querySelector(
      'input#rule-for-duration',
    ) as HTMLInputElement
    fireEvent.change(durationAmount, { target: { value: '10' } })

    fireEvent.click(within(popup).getByRole('button', { name: 'Create rule' }))

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        (c) => requestUrlOf(c[0] as RequestInfo | URL) === '/api/v1/apps/demo-app/alerts',
      )
      expect(call).toBeDefined()
    })

    const call = fetchMock.mock.calls.find(
      (c) => requestUrlOf(c[0] as RequestInfo | URL) === '/api/v1/apps/demo-app/alerts',
    )!
    const body = JSON.parse((call[1] as RequestInit).body as string) as {
      metric?: string
      for_duration?: string
    }
    expect(body.metric).toBe('memory_usage_bytes')
    expect(body.for_duration).toBe('10m')
  })
})
