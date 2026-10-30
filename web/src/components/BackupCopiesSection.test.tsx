import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BackupCopiesSection } from './BackupCopiesSection'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

const copy = (id: string, status: string) => ({
  backup_id: id,
  resource: id,
  resource_kind: 'database',
  source_target_id: 'a',
  target_id: 'b',
  status,
  attempts: 1,
  size_bytes: 1,
  backup_finished_at: '2026-10-28T00:00:00Z',
  lag_seconds: 5,
})

vi.mock('../queries/backupCopies', () => ({
  backupCopySettingsQueryOptions: () => ({
    queryKey: ['s'],
    queryFn: () => ({ enabled: true, target_id: 'b' }),
  }),
  backupCopyListQueryOptions: () => ({
    queryKey: ['c'],
    queryFn: () => [copy('ok', 'succeeded'), copy('bad', 'failed')],
  }),
  useSetBackupCopySettings: () => ({ mutate: vi.fn(), isPending: false }),
}))

describe('BackupCopiesSection', () => {
  it('shows the manifest indicator only on succeeded copies', async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <BackupCopiesSection targets={[]} />
      </QueryClientProvider>,
    )
    const marks = await screen.findAllByTestId('copy-manifest')
    expect(marks).toHaveLength(1)
  })
})
