import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { PendingChangesNotice } from './PendingChangesNotice'
import { jsonResponse, renderWithProviders } from '../stacks/stacksTestUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PendingChangesNotice', () => {
  it('shows nothing when no changes are pending', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        jsonResponse({ pending: false, changes: [], apply_action: 'restart' }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderWithProviders(<PendingChangesNotice appName="web" />)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('applies pending changes from the button', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'POST'
          ? jsonResponse({})
          : jsonResponse({
              pending: true,
              changes: [{ kind: 'env', keys: ['A', 'B'], since: 'now' }],
              apply_action: 'restart',
            }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderWithProviders(<PendingChangesNotice appName="web" />)
    fireEvent.click(await screen.findByRole('button'))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/apps/web/apply-pending', {
        method: 'POST',
      })
    })
  })
})
