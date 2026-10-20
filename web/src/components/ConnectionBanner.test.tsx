import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, onlineManager } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/apiError'
import {
  attachConnectionClient,
  reportError,
  resetConnectionForTests,
} from '../lib/connectionStore'
import { ConnectionBanner } from './ConnectionBanner'

function goOffline() {
  act(() => {
    reportError(new TypeError('Failed to fetch'))
    reportError(new ApiError(503, 'down'))
  })
}

describe('ConnectionBanner', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })
  afterEach(() => {
    resetConnectionForTests()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('renders nothing while connected', () => {
    render(<ConnectionBanner />)
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('ignores a single failure and non-connectivity errors', () => {
    render(<ConnectionBanner />)
    act(() => {
      reportError(new TypeError('x'))
      reportError(new ApiError(500, 'boom'))
    })
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('shows an alert with a live region and pauses polling when unreachable', () => {
    render(<ConnectionBanner />)
    goOffline()
    expect(screen.getByRole('alert')).toHaveTextContent(
      "Can't reach the control plane. Retrying...",
    )
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(onlineManager.isOnline()).toBe(false)
  })

  it('can be dismissed', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    render(<ConnectionBanner />)
    goOffline()
    await user.click(screen.getByRole('button', { name: 'Dismiss for now' }))
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('Retry now reconnects when /healthz answers and invalidates queries', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const client = new QueryClient()
    const invalidate = vi.spyOn(client, 'invalidateQueries')
    attachConnectionClient(client)
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('ok', { status: 200 })),
    )
    render(<ConnectionBanner />)
    goOffline()
    await user.click(screen.getByRole('button', { name: 'Retry now' }))
    expect(screen.queryByRole('alert')).toBeNull()
    expect(invalidate).toHaveBeenCalled()
    expect(onlineManager.isOnline()).toBe(true)
  })

  it('keeps the banner and backs off when the probe fails', async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('down')))
    render(<ConnectionBanner />)
    goOffline()
    await user.click(screen.getByRole('button', { name: 'Retry now' }))
    expect(screen.getByRole('alert')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('attempts so far: 1')
  })
})
