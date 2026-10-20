import { Suspense } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useDeployProgress } from './useDeployProgress'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'

const APP_NAME = 'web'

function requestUrlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function fakeJsonResponse(body: unknown): Response {
  return { ok: true, status: 200, json: () => Promise.resolve(body) } as unknown as Response
}

function makeAttempt(overrides: Partial<DeployAttempt> = {}): DeployAttempt {
  return {
    id: 'attempt-1',
    service_name: APP_NAME,
    image: 'myimage:v2',
    source: 'image',
    status: 'succeeded',
    started_at: '2026-01-01T00:00:00Z',
    finished_at: '2026-01-01T00:00:01Z',
    ...overrides,
  }
}

function makeCondition(overrides: Partial<ReconcileCondition> = {}): ReconcileCondition {
  return {
    Type: 'Ready',
    Status: 'True',
    Reason: 'Deploying',
    Message: '',
    LastTransitionTime: '2025-12-31T23:59:00Z',
    ...overrides,
  }
}

function ProbeComponent() {
  const { attempts, conditions } = useDeployProgress(APP_NAME)
  return (
    <div>
      <p>attempts:{attempts.length}</p>
      <p>condition-reason:{conditions[0]?.Reason ?? 'none'}</p>
    </div>
  )
}

function renderProbe() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={<div>loading</div>}>
        <ProbeComponent />
      </Suspense>
    </QueryClientProvider>,
  )
}

describe('useDeployProgress', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.useRealTimers()
  })

  it('polls while the rollout stage has not converged, and stops once it has', async () => {
    // recordInstantDeployAttempt (internal/api/deploys.go) marks a plain
    // image-tag redeploy 'succeeded' immediately, before the reconciler
    // has run even once: this attempt is already finished, but the one
    // condition on hand transitioned before finished_at, so
    // computeDeployStages' rollout stage reads 'running' (still
    // converging) until a fresher condition arrives.
    let conditions = [makeCondition()]

    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = requestUrlOf(input)
      if (url.includes('deploy-attempts')) {
        return Promise.resolve(fakeJsonResponse([makeAttempt()]))
      }
      return Promise.resolve(fakeJsonResponse(conditions))
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.useFakeTimers()

    renderProbe()

    await vi.waitFor(() => screen.getByText('attempts:1'))
    screen.getByText('condition-reason:Deploying')

    const callsBeforePoll = fetchMock.mock.calls.length
    expect(callsBeforePoll).toBeGreaterThanOrEqual(2)

    // The reconciler catches up: the next poll sees a fresh, terminal
    // condition (AlreadyRunning, one of ROLLOUT_DONE_REASONS).
    conditions = [
      makeCondition({ Reason: 'AlreadyRunning', LastTransitionTime: '2026-01-01T00:00:02Z' }),
    ]
    await vi.advanceTimersByTimeAsync(3_000)

    expect(fetchMock.mock.calls.length).toBeGreaterThan(callsBeforePoll)
    await vi.waitFor(() => screen.getByText('condition-reason:AlreadyRunning'))

    const callsAfterConverged = fetchMock.mock.calls.length

    // Converged: no further polling.
    await vi.advanceTimersByTimeAsync(10_000)
    expect(fetchMock.mock.calls.length).toBe(callsAfterConverged)
  })

  it('never polls when the latest attempt already converged on first load', async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = requestUrlOf(input)
      if (url.includes('deploy-attempts')) {
        return Promise.resolve(fakeJsonResponse([makeAttempt()]))
      }
      return Promise.resolve(
        fakeJsonResponse([makeCondition({ Reason: 'AlreadyRunning', LastTransitionTime: '2026-01-01T00:00:02Z' })]),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.useFakeTimers()

    renderProbe()

    await vi.waitFor(() => screen.getByText('attempts:1'))
    const callsAfterLoad = fetchMock.mock.calls.length

    await vi.advanceTimersByTimeAsync(10_000)
    expect(fetchMock.mock.calls.length).toBe(callsAfterLoad)
  })
})
