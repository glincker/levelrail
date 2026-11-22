import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AllI18n } from '../../test/allI18n'
import type { BulkRequest, BulkResponse } from '../../queries/appsBulk'
import { BulkActionsBar } from './BulkActionsBar'

const postBulk = vi.fn<(req: BulkRequest) => Promise<BulkResponse>>()
vi.mock('../../queries/appsBulk', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../queries/appsBulk')>()),
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

  it('scales with a replica count or an autoscale range from one field', async () => {
    postBulk.mockImplementation((req) =>
      Promise.resolve({
        action: req.action,
        dry_run: req.dry_run ?? false,
        counts: {},
        results: [{ name: 'web', status: 'would_apply' }],
      }),
    )
    renderBar()
    fireEvent.change(screen.getByLabelText('More actions'), {
      target: { value: 'scale' },
    })
    const field = screen.getByLabelText('Replicas (3) or min-max (2-6)')
    const review = screen.getByRole('button', { name: 'Review' })
    fireEvent.change(field, { target: { value: 'many' } })
    expect(review).toHaveProperty('disabled', true)
    fireEvent.change(field, { target: { value: '2-6' } })
    fireEvent.click(review)
    await waitFor(() => {
      expect(postBulk).toHaveBeenCalledWith(
        expect.objectContaining({
          action: 'scale',
          min_replicas: 2,
          max_replicas: 6,
          dry_run: true,
        }),
      )
    })
  })

  it('sets memory from one field and shows each app before and after', async () => {
    postBulk.mockImplementation((req) =>
      Promise.resolve({
        action: req.action,
        dry_run: req.dry_run ?? false,
        counts: {},
        results: [
          {
            name: 'web',
            status: 'would_apply',
            before: '256 MiB',
            after: '512 MiB',
          },
        ],
      }),
    )
    renderBar()
    fireEvent.change(screen.getByLabelText('More actions'), {
      target: { value: 'set-memory' },
    })
    const field = screen.getByLabelText('Memory (512Mi or 2Gi)')
    const review = screen.getByRole('button', { name: 'Review' })
    fireEvent.change(field, { target: { value: '512' } })
    expect(review).toHaveProperty('disabled', true)
    fireEvent.change(field, { target: { value: '512Mi' } })
    fireEvent.click(review)
    await screen.findByText('256 MiB to 512 MiB')
    expect(postBulk).toHaveBeenCalledWith(
      expect.objectContaining({
        action: 'set-memory',
        memory_bytes: 536870912,
        dry_run: true,
      }),
    )
  })
})
