import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AllI18n } from '../../test/allI18n'
import type { BulkRequest, BulkResponse } from '../../queries/appsBulk'
import { BulkActionsBar } from './BulkActionsBar'

const postBulk = vi.fn<(req: BulkRequest) => Promise<BulkResponse>>()
vi.mock('../../queries/appsBulk', () => ({
  postBulk: (req: BulkRequest) => postBulk(req),
}))

function renderBar(canAct = true) {
  const qc = new QueryClient()
  return render(
    <AllI18n>
      <QueryClientProvider client={qc}>
        <BulkActionsBar
          selected={['web', 'api']}
          canAct={canAct}
          onClear={() => undefined}
        />
      </QueryClientProvider>
    </AllI18n>,
  )
}

describe('BulkActionsBar', () => {
  beforeEach(() => {
    postBulk.mockReset()
  })

  it('requires the typed count, then lists per-app errors on partial failure', async () => {
    postBulk.mockImplementation((req) =>
      Promise.resolve({
        action: req.action,
        dry_run: req.dry_run ?? false,
        counts: {},
        results: req.dry_run
          ? [
              { name: 'web', status: 'would_apply' },
              { name: 'api', status: 'would_apply' },
            ]
          : [
              { name: 'web', status: 'ok' },
              { name: 'api', status: 'error', message: 'node offline' },
            ],
      }),
    )
    renderBar()
    fireEvent.click(screen.getByRole('button', { name: 'Restart' }))
    const confirm = await screen.findByRole('button', { name: 'Restart 2' })
    expect(confirm).toHaveProperty('disabled', true)
    fireEvent.change(screen.getByLabelText('Type 2 to confirm'), {
      target: { value: '2' },
    })
    expect(confirm).toHaveProperty('disabled', false)
    fireEvent.click(confirm)
    await screen.findByText('node offline')
    expect(screen.getByText('Failed')).toBeTruthy()
    await waitFor(() => {
      expect(postBulk).toHaveBeenLastCalledWith(
        expect.objectContaining({ confirm_count: 2, names: ['web', 'api'] }),
      )
    })
  })

  it('disables actions for a read-only user with one line', () => {
    renderBar(false)
    expect(screen.getByRole('button', { name: 'Restart' })).toHaveProperty(
      'disabled',
      true,
    )
    expect(screen.getByText(/read-only/)).toBeTruthy()
  })
})
