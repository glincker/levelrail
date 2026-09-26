import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, renderHook, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { makeDeployment } from '../../test/deploymentFixtures'
import { useDeploymentActions } from '../../hooks/useDeploymentActions'
import { cancelReason } from '../../lib/deploymentReasons'
import { confirmCopy } from '../../lib/deploymentConfirmCopy'
import { subtitle } from '../../lib/deploymentPresentation'
import { applyEventToLane } from '../../lib/deploymentsCache'
import { BuildingNowLane } from './BuildingNowLane'
import { DeploymentNote } from './DeploymentNote'

const toastAdd = vi.hoisted(() => vi.fn())
vi.mock('@/components/ui/toast', () => ({ toast: { add: toastAdd } }))

const queued = makeDeployment({
  id: 'q-2222222222',
  status: 'queued',
  queue_position: 2,
  wait_reason: 'waiting for the running deploy',
  blocked_by: 'b-1111111111',
  finished_at: null,
  duration_ms: null,
})

describe('row states', () => {
  it('describes a queued deploy with its place and reason', () => {
    expect(subtitle(queued)).toBe('Queued #2, waiting for the running deploy')
  })

  it('names who canceled', () => {
    const d = makeDeployment({ status: 'canceled', canceled_by: 'ada' })
    expect(subtitle(d)).toMatch(/^Canceled by ada, /)
  })

  it('explains a superseded row', () => {
    const d = makeDeployment({ status: 'superseded', superseded_by: 'w-9999' })
    expect(subtitle(d)).toContain('Superseded by w-9999')
  })

  it('explains cancel being unavailable', () => {
    expect(cancelReason(makeDeployment({ is_live: true }))).toMatch(/live/)
    expect(cancelReason(makeDeployment())).toMatch(/already finished/)
    expect(cancelReason(queued)).toBe('')
  })

  it('states environment and digest in the rollback copy', () => {
    const copy = confirmCopy({
      kind: 'rollback',
      deployment: makeDeployment(),
    })
    expect(copy.description).toContain('production')
    expect(copy.description).toContain('sha256:abcdef0123456789abcdef')
  })
})

describe('DeploymentNote', () => {
  it('links a queued deploy to the deploy blocking it', async () => {
    const onOpen = vi.fn()
    render(<DeploymentNote d={queued} onOpen={onOpen} />)
    await userEvent.click(screen.getByRole('button', { name: 'b-111111' }))
    expect(onOpen).toHaveBeenCalledWith('b-1111111111')
  })

  it('links a superseded deploy to the winner', async () => {
    const onOpen = vi.fn()
    const d = makeDeployment({
      status: 'superseded',
      superseded_by: 'winner-01',
    })
    render(<DeploymentNote d={d} onOpen={onOpen} />)
    await userEvent.click(screen.getByRole('button', { name: 'winner-0' }))
    expect(onOpen).toHaveBeenCalledWith('winner-01')
  })

  it('shows who canceled and when', () => {
    const d = makeDeployment({ status: 'canceled', canceled_by: 'ada' })
    render(<DeploymentNote d={d} onOpen={vi.fn()} />)
    expect(screen.getByText(/Canceled by ada/)).toBeInTheDocument()
    expect(document.querySelector('time')).not.toBeNull()
  })
})

describe('BuildingNowLane', () => {
  it('shows the queue label and enables cancel for queued rows', async () => {
    const onCancel = vi.fn()
    render(
      <BuildingNowLane
        rows={[queued]}
        now={0}
        onOpen={vi.fn()}
        onCancel={onCancel}
      />,
    )
    expect(
      screen.getByText(/Queued #2, waiting for the running deploy/),
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onCancel).toHaveBeenCalledWith(queued)
  })

  it('disables cancel with a reason once a deploy is live', () => {
    const live = makeDeployment({ status: 'building', is_live: true })
    render(
      <BuildingNowLane
        rows={[{ ...live, status: 'ready' }]}
        now={0}
        onOpen={vi.fn()}
        onCancel={vi.fn()}
      />,
    )
    const btn = screen.getByRole('button', { name: 'Cancel' })
    expect(btn).toBeDisabled()
    expect(btn).toHaveAttribute('title', expect.stringMatching(/live/))
  })
})

describe('lane queue transitions', () => {
  it('orders building before the queue, then by position', () => {
    const building = makeDeployment({ id: 'b', status: 'building' })
    const q1 = { ...queued, id: 'q1', queue_position: 1 }
    const q2 = { ...queued, id: 'q2', queue_position: 2 }
    let rows = applyEventToLane([], { type: 'created', deployment: q2 })
    rows = applyEventToLane(rows, { type: 'created', deployment: q1 })
    rows = applyEventToLane(rows, { type: 'created', deployment: building })
    expect(rows.map((r) => r.id)).toEqual(['b', 'q1', 'q2'])
  })

  it('moves a queued row to building in place and drops it once canceled', () => {
    const rows = [queued]
    const started = {
      ...queued,
      status: 'building' as const,
      queue_position: null,
    }
    const building = applyEventToLane(rows, {
      type: 'step',
      deployment: started,
    })
    expect(building).toHaveLength(1)
    expect(building[0]?.status).toBe('building')
    const canceled = applyEventToLane(building, {
      type: 'finished',
      deployment: { ...started, status: 'canceled', canceled_by: 'ada' },
    })
    expect(canceled).toEqual([])
  })
})

function jsonRes(status: number, body: unknown) {
  return Promise.resolve(new Response(JSON.stringify(body), { status }))
}

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  )
}

describe('useDeploymentActions', () => {
  beforeEach(() => {
    toastAdd.mockClear()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  async function run(
    kind: 'cancel' | 'rollback',
    d: ReturnType<typeof makeDeployment>,
    respond: () => Promise<Response>,
    onViewApproval?: () => void,
  ) {
    const fetchMock = vi.fn<(url: string) => Promise<Response>>(() => respond())
    vi.stubGlobal('fetch', fetchMock)
    const { result } = renderHook(() => useDeploymentActions(onViewApproval), {
      wrapper: wrapper(),
    })
    act(() => {
      result.current.request(kind, d)
    })
    await act(async () => {
      result.current.confirm()
      await vi.waitFor(() => {
        expect(toastAdd).toHaveBeenCalled()
      })
    })
    return { fetchMock, result }
  }

  it('shows the server reason when cancel answers 409', async () => {
    const { fetchMock } = await run('cancel', queued, () =>
      jsonRes(409, { error: 'this deploy already cut over' }),
    )
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/v1/apps/web/deploys/q-2222222222/cancel',
    )
    expect(toastAdd).toHaveBeenCalledWith({
      title: 'this deploy already cut over',
      type: 'error',
    })
  })

  it('shows the server reason when rollback answers 410', async () => {
    const d = makeDeployment({ id: 'r1' })
    await run('rollback', d, () =>
      jsonRes(410, { error: 'image was garbage collected' }),
    )
    expect(toastAdd).toHaveBeenCalledWith({
      title: 'image was garbage collected',
      type: 'error',
    })
  })

  it('shows the server reason when rollback answers 409', async () => {
    await run('rollback', makeDeployment({ id: 'r1' }), () =>
      jsonRes(409, { error: 'tag moved' }),
    )
    expect(toastAdd).toHaveBeenCalledWith({ title: 'tag moved', type: 'error' })
  })

  it('links to the approval when rollback needs one', async () => {
    const onViewApproval = vi.fn()
    const { fetchMock } = await run(
      'rollback',
      makeDeployment({ id: 'r1' }),
      () => jsonRes(202, { pending_approval: { id: 'ap1' } }),
      onViewApproval,
    )
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/v1/apps/web/deploys/r1/rollback',
    )
    const arg = toastAdd.mock.calls[0]?.[0] as {
      title: string
      type: string
      actionProps?: { children: string; onClick: () => void }
    }
    expect(arg.title).toMatch(/waiting for approval/)
    expect(arg.actionProps?.children).toBe('View approval')
    arg.actionProps?.onClick()
    expect(onViewApproval).toHaveBeenCalled()
  })
})
