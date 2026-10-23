import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/apiError'
import { renderInRouter } from '../test/renderInRouter'
import { RouteErrorFallback } from './RouteErrorFallback'

function mount(error: unknown) {
  return renderInRouter(<RouteErrorFallback error={error} reset={vi.fn()} />)
}

describe('RouteErrorFallback', () => {
  it('shows the not-found state for a 404', async () => {
    await mount(new ApiError(404, 'app not found: x'))
    expect(await screen.findByText('Page not found')).toBeTruthy()
    expect(screen.queryByText('Something went wrong')).toBeNull()
  })

  it('keeps the forbidden state for a 403', async () => {
    await mount(new ApiError(403, 'not allowed'))
    expect(await screen.findByText('not allowed')).toBeTruthy()
    expect(screen.queryByText('Something went wrong')).toBeNull()
  })

  it('offers retry, request id and copy for a 503', async () => {
    await mount(
      new ApiError(503, 'unavailable', { code: 'x', requestId: 'req-42' }),
    )
    expect(
      await screen.findByText("Can't reach the control plane"),
    ).toBeTruthy()
    expect(screen.getByText('Request ID: req-42')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Copy details/ })).toBeTruthy()
  })

  it('treats a network TypeError as unreachable', async () => {
    await mount(new TypeError('Failed to fetch'))
    expect(
      await screen.findByText("Can't reach the control plane"),
    ).toBeTruthy()
  })

  it('keeps the technical message under Details for unknown errors', async () => {
    await mount(new Error('boom internals'))
    expect(await screen.findByText('Something went wrong')).toBeTruthy()
    expect(screen.getByText('Details')).toBeTruthy()
    expect(screen.getByText('boom internals')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy()
  })
})
