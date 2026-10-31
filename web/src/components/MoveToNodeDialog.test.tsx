import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../locales/en/nodes.json'
import { MoveToNodeDialog } from './MoveToNodeDialog'
import type { NodeResource } from '../types/nodeDetail'
import type { NodeMove } from '../types/nodeMoves'
import { createTestQueryClient } from '@/test/testQueryClient'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function mockFetchRoutes(
  routes: Record<string, () => Response>,
): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const path = requestUrlOf(input).split('?')[0]
    const handler = routes[`${method} ${path}`]
    if (handler) return Promise.resolve(handler())
    return Promise.reject(new Error(`unexpected fetch: ${method} ${path}`))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

const localNode: NodeResource = {
  id: 'local-1',
  name: 'primary',
  status: 'online',
  schedulable: true,
  accepts_app_workloads: true,
  accepts_build_workloads: true,
  created_at: '2026-01-01T00:00:00Z',
  is_local: true,
}

const workerNode: NodeResource = {
  ...localNode,
  id: 'node-1',
  name: 'worker-1',
  is_local: false,
}

function baseMove(over: Partial<NodeMove>): NodeMove {
  return {
    id: 'mv_1',
    kind: 'app',
    resource: 'web',
    from_node_id: 'local-1',
    to_node_id: 'node-1',
    stateful: false,
    volumes: [],
    phase: 'preflight',
    status: 'running',
    steps: [],
    started_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...over,
  }
}

function renderDialog(props: { volumeNames?: string[] } = {}) {
  const queryClient = createTestQueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <MoveToNodeDialog kind="app" name="web" {...props} />
    </QueryClientProvider>,
  )
}

async function openDialog() {
  fireEvent.click(screen.getByText('Move'))
  await screen.findByText('Move to')
}

function selectItems(): (string | null)[] {
  return Array.from(
    document.body.querySelectorAll('[data-slot="select-item"]'),
  ).map((el) => el.textContent)
}

function submitButton(): Element {
  const match = screen.getAllByText('Move').at(-1)
  if (!match) throw new Error('no Move button')
  return match
}

describe('MoveToNodeDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('offers automatic placement and the other nodes', async () => {
    mockFetchRoutes({
      'GET /api/v1/nodes': () => jsonResponse([localNode, workerNode]),
    })
    renderDialog()
    await openDialog()
    expect(
      screen.getAllByText('Automatic (least loaded eligible node)').length,
    ).toBeGreaterThan(0)
    fireEvent.click(document.querySelector('#move-target-node') as Element)
    await waitFor(() => {
      expect(selectItems().some((text) => text?.includes('worker-1'))).toBe(
        true,
      )
    })
    expect(selectItems().some((text) => text?.includes('primary'))).toBe(false)
  })

  it('warns about downtime when the app has volumes', async () => {
    mockFetchRoutes({
      'GET /api/v1/nodes': () => jsonResponse([localNode, workerNode]),
    })
    renderDialog({ volumeNames: ['data'] })
    await openDialog()
    expect(
      await screen.findByText(/is stopped while they are copied/),
    ).toBeVisible()
  })

  it('says so when there is no other node', async () => {
    mockFetchRoutes({ 'GET /api/v1/nodes': () => jsonResponse([localNode]) })
    renderDialog()
    fireEvent.click(screen.getByText('Move'))
    expect(
      await screen.findByText(/no other nodes to move this to/i),
    ).toBeVisible()
  })

  it('starts an automatic move and shows completion', async () => {
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/nodes': () => jsonResponse([localNode, workerNode]),
      'POST /api/v1/apps/web/move': () => jsonResponse(baseMove({}), 202),
      'GET /api/v1/node-moves/mv_1': () =>
        jsonResponse(
          baseMove({
            status: 'succeeded',
            phase: 'done',
            steps: [
              {
                phase: 'verify',
                outcome: 'ok',
                at: '2026-01-01T00:00:01Z',
              },
            ],
          }),
        ),
    })
    renderDialog()
    await openDialog()
    fireEvent.click(submitButton())

    expect(await screen.findByText('Move complete')).toBeVisible()
    expect(screen.getByText('Verifying health')).toBeVisible()
    expect(screen.getByText('Done')).toBeVisible()
    const post = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    )
    expect(JSON.parse((post?.[1] as RequestInit).body as string)).toEqual({
      node_id: '',
    })
  })

  it('sends the chosen node id', async () => {
    const fetchMock = mockFetchRoutes({
      'GET /api/v1/nodes': () => jsonResponse([localNode, workerNode]),
      'POST /api/v1/apps/web/move': () => jsonResponse(baseMove({}), 202),
      'GET /api/v1/node-moves/mv_1': () => jsonResponse(baseMove({})),
    })
    renderDialog()
    await openDialog()
    fireEvent.click(document.querySelector('#move-target-node') as Element)
    await waitFor(() => {
      expect(selectItems().some((text) => text?.includes('worker-1'))).toBe(
        true,
      )
    })
    const option = Array.from(
      document.body.querySelectorAll('[data-slot="select-item"]'),
    ).find((el) => el.textContent?.includes('worker-1'))
    if (!option) throw new Error('worker-1 option not found')
    await userEvent.setup().click(option)
    fireEvent.click(submitButton())

    await screen.findByText('Moving')
    const post = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    )
    expect(JSON.parse((post?.[1] as RequestInit).body as string)).toEqual({
      node_id: 'node-1',
    })
  })

  it('shows the error and a retry hint when the move is rolled back', async () => {
    mockFetchRoutes({
      'GET /api/v1/nodes': () => jsonResponse([localNode, workerNode]),
      'POST /api/v1/apps/web/move': () => jsonResponse(baseMove({}), 202),
      'GET /api/v1/node-moves/mv_1': () =>
        jsonResponse(
          baseMove({
            status: 'rolled_back',
            phase: 'rollback',
            error: 'target never became healthy',
          }),
        ),
    })
    renderDialog()
    await openDialog()
    fireEvent.click(submitButton())

    expect(await screen.findByText('target never became healthy')).toBeVisible()
    expect(screen.getByText(/is still on its original node/)).toBeVisible()
  })
})
