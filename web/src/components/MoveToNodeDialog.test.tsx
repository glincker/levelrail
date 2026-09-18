import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MoveToNodeDialog } from './MoveToNodeDialog'
import type { NodeResource } from '../types/nodeDetail'
import type { AppVolumeMove } from '../types/appVolumeMove'

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

// Same route-table stub PromoteAppDialog.test.tsx already establishes:
// each test states which endpoints it needs and what they return, keyed
// on method+path. A handler is a function so a test can hand back a
// different body on a second call (the poll-until-done tests).
function mockFetchRoutes(
  routes: Record<string, () => Promise<Response>>,
): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = requestUrlOf(input)
    const method = init?.method ?? 'GET'
    const path = url.split('?')[0]
    const handler = routes[`${method} ${path}`]
    if (handler) return handler()
    return Promise.reject(new Error(`unexpected fetch: ${method} ${url}`))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function jsonRoute(body: unknown, status = 200): () => Promise<Response> {
  return () => Promise.resolve(fakeJsonResponse(body, status))
}

const nodeOne: NodeResource = {
  id: 'node-1',
  name: 'worker-1',
  status: 'online',
  schedulable: true,
  accepts_app_workloads: true,
  accepts_build_workloads: true,
  created_at: '2026-01-01T00:00:00Z',
}

function renderDialog(volumeCount: number) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <MoveToNodeDialog kind="app" name="web" volumeCount={volumeCount} />
    </QueryClientProvider>,
  )
}

// Disambiguates "Move" the dialog trigger from "Move" the footer submit
// button: both are on screen at once once the dialog is open, and
// screen.getByText('Move') alone is ambiguous between them. The submit
// button is the later one in DOM order (rendered inside the dialog
// portal, appended after the trigger).
function clickMoveSubmitButton() {
  const matches = screen.getAllByText('Move')
  const submitButton = matches.at(-1)
  if (!submitButton) throw new Error('no "Move" button found')
  fireEvent.click(submitButton)
}

async function openDialogAndPickNode() {
  fireEvent.click(screen.getByText('Move'))
  await screen.findByText('Move to')
  fireEvent.click(document.querySelector('#move-target-node') as Element)
  const user = userEvent.setup()
  const option = Array.from(
    document.body.querySelectorAll('[data-slot="select-item"]'),
  ).find((el) => el.textContent?.includes('worker-1'))
  if (!option) throw new Error('worker-1 option not found')
  await user.click(option)
}

describe('MoveToNodeDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('does not offer "take its volumes with it" when the app has no volumes', async () => {
    mockFetchRoutes({ 'GET /api/v1/nodes': jsonRoute([nodeOne]) })
    renderDialog(0)
    fireEvent.click(screen.getByText('Move'))
    await screen.findByText('Move to')
    expect(screen.queryByText(/Take .* with it/)).not.toBeInTheDocument()
  })

  it('offers "take its volumes with it" when the app has named volumes', async () => {
    mockFetchRoutes({ 'GET /api/v1/nodes': jsonRoute([nodeOne]) })
    renderDialog(2)
    fireEvent.click(screen.getByText('Move'))
    await screen.findByText('Move to')
    expect(screen.getByText('Take its volumes with it')).toBeInTheDocument()
  })

  it('shows a warning once the volumes checkbox is checked', async () => {
    mockFetchRoutes({ 'GET /api/v1/nodes': jsonRoute([nodeOne]) })
    renderDialog(1)
    fireEvent.click(screen.getByText('Move'))
    await screen.findByText('Take its volume with it')
    fireEvent.click(screen.getByRole('checkbox'))
    expect(
      await screen.findByText(/will be stopped for the duration/),
    ).toBeInTheDocument()
  })

  it('calls move-with-volumes instead of the plain node PUT when checked, and shows Done once succeeded', async () => {
    const finishedMove: AppVolumeMove = {
      id: 'avm_1',
      service_name: 'web',
      from_node_id: '',
      to_node_id: 'node-1',
      status: 'succeeded',
      steps: [
        {
          name: 'update_placement',
          status: 'succeeded',
          started_at: '2026-01-01T00:00:00Z',
          finished_at: '2026-01-01T00:00:01Z',
        },
      ],
      started_at: '2026-01-01T00:00:00Z',
      finished_at: '2026-01-01T00:00:01Z',
    }
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/nodes': jsonRoute([nodeOne]),
      'POST /api/v1/apps/web/move-with-volumes': jsonRoute(finishedMove),
      // TanStack Query's default refetchOnMount still fires GetAppVolumeMove
      // once even though the POST response already carries a finished
      // record: same shape as the "running" case, just already succeeded.
      'GET /api/v1/apps/web/moves/avm_1': jsonRoute(finishedMove),
    })
    renderDialog(1)
    await openDialogAndPickNode()
    fireEvent.click(screen.getByRole('checkbox'))
    clickMoveSubmitButton()

    await screen.findByText('Done')
    expect(
      fetchMock.mock.calls.some(([input, init]) =>
        requestUrlOf(input as RequestInfo | URL).endsWith(
          '/api/v1/apps/web/move-with-volumes',
        ) && (init as RequestInit | undefined)?.method === 'POST',
      ),
    ).toBe(true)
    expect(
      fetchMock.mock.calls.some(
        ([, init]) => (init as RequestInit | undefined)?.method === 'PUT',
      ),
    ).toBe(false)
  })

  it('polls GetAppVolumeMove while running and shows the failure inline once it fails', async () => {
    let pollCalls = 0
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/nodes': jsonRoute([nodeOne]),
      'POST /api/v1/apps/web/move-with-volumes': jsonRoute({
        id: 'avm_1',
        service_name: 'web',
        from_node_id: '',
        to_node_id: 'node-1',
        status: 'running',
        steps: [],
        started_at: '2026-01-01T00:00:00Z',
      } satisfies AppVolumeMove),
      'GET /api/v1/apps/web/moves/avm_1': () => {
        pollCalls += 1
        const status = pollCalls >= 2 ? 'failed' : 'running'
        return Promise.resolve(
          fakeJsonResponse({
            id: 'avm_1',
            service_name: 'web',
            from_node_id: '',
            to_node_id: 'node-1',
            status,
            error: status === 'failed' ? 'docker daemon unreachable' : undefined,
            steps: [
              {
                name: 'move_volume:app-web-data',
                status,
                started_at: '2026-01-01T00:00:00Z',
              },
            ],
            started_at: '2026-01-01T00:00:00Z',
          } satisfies AppVolumeMove),
        )
      },
    })
    renderDialog(1)
    await openDialogAndPickNode()
    fireEvent.click(screen.getByRole('checkbox'))
    clickMoveSubmitButton()

    await waitFor(() => expect(pollCalls).toBeGreaterThanOrEqual(2), {
      timeout: 5000,
    })
    expect(
      await screen.findByText('docker daemon unreachable'),
    ).toBeInTheDocument()
    // At least two "Close" texts exist once a move has failed: this
    // dialog's own Cancel-button-turned-Close, and the sr-only label on
    // DialogContent's built-in X button (dialog.tsx). getAllByText
    // sidesteps the ambiguity a plain getByText would hit here.
    expect(screen.getAllByText('Close').length).toBeGreaterThanOrEqual(2)
    void fetchMock
  })
})
