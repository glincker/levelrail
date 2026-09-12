import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EditAlertRuleDialog } from './EditAlertRuleDialog'
import type { AlertRule } from './../types/alerts'

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

const thresholdRule: AlertRule = {
  id: 'rule_1',
  name: 'high-cpu',
  kind: 'threshold',
  resource_id: 'app:demo-app',
  metric: 'cpu_percent',
  comparator: '>',
  threshold: 80,
  for_duration: '90s',
  restart_count_threshold: 0,
  enabled: true,
}

function renderDialog(rule: AlertRule = thresholdRule) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <EditAlertRuleDialog appName="demo-app" rule={rule} />
    </QueryClientProvider>,
  )
}

// The dialog popup renders through a portal, outside RTL's own
// `container` root, so every subsequent lookup below is scoped to the
// popup element this returns (or to `screen`/`document`), never to
// RTL's `container`.
async function openDialog() {
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
  await screen.findByText('Edit alert rule')
  const popup = document.querySelector('[data-slot="dialog-content"]')
  if (!popup) throw new Error('dialog content did not render')
  return popup as HTMLElement
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

describe('EditAlertRuleDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('parses an existing for_duration string into the DurationInput amount+unit', async () => {
    mockFetchRoutes({
      'GET /api/v1/notification-channels': jsonRoute([]),
      'GET /api/v1/apps/demo-app/scheduled-tasks': jsonRoute([]),
    })
    renderDialog()
    const popup = await openDialog()

    const durationAmount = popup.querySelector(
      'input#edit-rule-for-duration',
    ) as HTMLInputElement
    expect(durationAmount.value).toBe('90')
    expect(screen.getByText('Seconds')).toBeInTheDocument()
  })

  it('renders the metric field as a closed enum select, not free text', async () => {
    mockFetchRoutes({
      'GET /api/v1/notification-channels': jsonRoute([]),
      'GET /api/v1/apps/demo-app/scheduled-tasks': jsonRoute([]),
    })
    renderDialog()
    const popup = await openDialog()

    expect(
      popup.querySelector('input#edit-rule-metric'),
    ).not.toBeInTheDocument()
    const trigger = popup.querySelector('#edit-rule-metric')
    expect(trigger).toBeInTheDocument()
    expect(trigger?.tagName).toBe('BUTTON')

    fireEvent.click(trigger!)
    expect(await screen.findByText('Memory usage')).toBeInTheDocument()
    expect(screen.getByText('Network received')).toBeInTheDocument()
  })

  it('submits a changed metric and a re-composed for_duration string as a full replace', async () => {
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/notification-channels': jsonRoute([]),
      'GET /api/v1/apps/demo-app/scheduled-tasks': jsonRoute([]),
      'PUT /api/v1/apps/demo-app/alerts/rule_1': jsonRoute({
        ...thresholdRule,
        metric: 'memory_usage_bytes',
        for_duration: '5m',
      }),
    })
    renderDialog()
    const popup = await openDialog()

    await pickOption(popup, 'edit-rule-metric', 'Memory usage')

    const durationAmount = popup.querySelector(
      'input#edit-rule-for-duration',
    ) as HTMLInputElement
    fireEvent.change(durationAmount, { target: { value: '5' } })
    await pickOption(popup, 'edit-rule-for-duration-unit', 'Minutes')

    fireEvent.click(within(popup).getByRole('button', { name: 'Save changes' }))

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        (c) =>
          requestUrlOf(c[0] as RequestInfo | URL) ===
          '/api/v1/apps/demo-app/alerts/rule_1',
      )
      expect(call).toBeDefined()
    })

    const call = fetchMock.mock.calls.find(
      (c) =>
        requestUrlOf(c[0] as RequestInfo | URL) ===
        '/api/v1/apps/demo-app/alerts/rule_1',
    )!
    const body = JSON.parse((call[1] as RequestInit).body as string) as {
      metric?: string
      for_duration?: string
    }
    expect(body.metric).toBe('memory_usage_bytes')
    expect(body.for_duration).toBe('5m')
  })
})
