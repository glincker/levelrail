import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { StackDetail } from './StackDetail'
import {
  jsonResponse,
  renderWithProviders,
  revision,
  stack,
} from './stacksTestUtils'
import type { Stack } from '../../types/stacks'

afterEach(() => {
  vi.unstubAllGlobals()
})

function mockApi(s: Stack) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    if (url.includes('/rollback')) {
      return Promise.resolve(
        jsonResponse({
          stack: s.name,
          revision: revision(4, { status: 'applied', rollback_of: 2 }),
        }),
      )
    }
    if (url.includes('/revisions')) {
      return Promise.resolve(
        jsonResponse({
          stack: s.name,
          revisions: [revision(3), revision(2), revision(1)],
        }),
      )
    }
    if (method === 'GET') return Promise.resolve(jsonResponse(s))
    return Promise.resolve(jsonResponse({}))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderDetail(s: Stack, isRoot: boolean) {
  return renderWithProviders(
    <StackDetail
      name={s.name}
      isRoot={isRoot}
      selection={{ diff: false, against: 'live' }}
      onSelect={vi.fn()}
      onBack={vi.fn()}
    />,
  )
}

describe('StackDetail', () => {
  it('lists the revision history newest first', async () => {
    mockApi(stack())
    renderDetail(stack(), true)
    expect(await screen.findAllByText('r3')).not.toHaveLength(0)
    expect(screen.getAllByText('r1').length).toBeGreaterThan(0)
  })

  it('confirms a rollback and says secrets and data are not rolled back', async () => {
    const fetchMock = mockApi(stack())
    renderDetail(stack(), false)
    fireEvent.click(await screen.findByLabelText('Roll back to revision 2'))
    expect(
      await screen.findByText(/Secrets are not rolled back/),
    ).toBeInTheDocument()
    expect(screen.getByText(/Data is not rolled back/)).toBeInTheDocument()
    expect(
      fetchMock.mock.calls.some(([u]) => String(u).includes('/rollback')),
    ).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Roll back' }))
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([u]) =>
          String(u).endsWith('/revisions/2/rollback'),
        ),
      ).toBe(true)
    })
  })

  it('hides unlock and disables settings for a non-admin', async () => {
    const locked = stack({
      lock: { holder: 'ci', revision: 3, expires_at: '2026-10-25T00:00:00Z' },
    })
    mockApi(locked)
    renderDetail(locked, false)
    await screen.findByText('Stack settings')
    expect(screen.queryByRole('button', { name: 'Unlock' })).toBeNull()
    expect(screen.getByRole('switch', { name: 'Allow prune' })).toHaveAttribute(
      'data-disabled',
    )
  })

  it('shows unlock to an admin', async () => {
    const locked = stack({
      lock: { holder: 'ci', revision: 3, expires_at: '2026-10-25T00:00:00Z' },
    })
    mockApi(locked)
    renderDetail(locked, true)
    expect(
      await screen.findByRole('button', { name: 'Unlock' }),
    ).toBeInTheDocument()
  })

  it('offers destroy on delete only to an admin', async () => {
    mockApi(stack())
    const { unmount } = renderDetail(stack(), false)
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('Forget the stack')).toBeInTheDocument()
    expect(screen.queryByText('Destroy resources')).toBeNull()
    unmount()

    renderDetail(stack(), true)
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('Destroy resources')).toBeInTheDocument()
  })
})
