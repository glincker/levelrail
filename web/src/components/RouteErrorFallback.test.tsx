import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/apiError'
import { errorReport } from '../lib/errorReport'
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

describe('RouteErrorFallback crash screen', () => {
  it('announces the error and focuses the heading', async () => {
    await mount(new Error('boom'))
    const heading = await screen.findByRole('heading', {
      name: 'Something went wrong',
    })
    expect(screen.getByRole('alert')).toBeTruthy()
    expect(document.activeElement).toBe(heading)
    expect(screen.getByRole('button', { name: /Report an issue/ })).toBeTruthy()
  })
})

describe('errorReport', () => {
  it('is small, has the context and carries no secrets', () => {
    const err = new Error('failed with token=abc123secretvalue')
    err.stack = Array.from(
      { length: 40 },
      (_, i) => `at fn${i} (x.js:${i})`,
    ).join('\n')
    const text = errorReport(err, 'Error report', '1.1.0')
    expect(text).toContain('version: 1.1.0')
    expect(text).toContain('route:')
    expect(text).toContain('browser:')
    expect(text).not.toContain('abc123secretvalue')
    expect(text).not.toContain('fn20')
    expect(text.length).toBeLessThan(1500)
  })
})
