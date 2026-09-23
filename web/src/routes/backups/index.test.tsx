import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { allBackupHistoryQueryOptions } from '../../queries/allBackupHistory'
import { shouldPromptForBackupTarget, Route } from './index'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ to, children }: { to?: string; children?: ReactNode }) => (
      <a href={to}>{children}</a>
    ),
  }
})

vi.mock('../../queries/backupTargets', () => ({
  useBackupTargetsOptional: vi.fn(),
}))

import { useBackupTargetsOptional } from '../../queries/backupTargets'

describe('shouldPromptForBackupTarget', () => {
  it('is false while the targets query is still loading, even with zero so far', () => {
    expect(shouldPromptForBackupTarget(true, 0)).toBe(false)
  })

  it('is true once loaded with no targets configured', () => {
    expect(shouldPromptForBackupTarget(false, 0)).toBe(true)
  })

  it('is false once loaded with at least one target configured', () => {
    expect(shouldPromptForBackupTarget(false, 1)).toBe(false)
  })
})

function renderBackupsPage() {
  const queryClient = new QueryClient()
  queryClient.setQueryData(allBackupHistoryQueryOptions().queryKey, [])
  const AllBackupsPage = Route.options.component!
  return render(
    <QueryClientProvider client={queryClient}>
      <AllBackupsPage />
    </QueryClientProvider>,
  )
}

describe('AllBackupsPage empty state', () => {
  it('prompts to add a backup target when history and targets are both empty', () => {
    vi.mocked(useBackupTargetsOptional).mockReturnValue({
      data: [],
      isLoading: false,
    } as unknown as ReturnType<typeof useBackupTargetsOptional>)

    renderBackupsPage()

    expect(screen.getByText('No backup target connected')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: 'Add a backup target' })
    expect(link).toHaveAttribute('href', '/settings/backup-targets')
  })

  it('shows a plain empty state, no prerequisite CTA, once a target exists', () => {
    vi.mocked(useBackupTargetsOptional).mockReturnValue({
      data: [
        {
          id: 't1',
          name: 'Primary bucket',
          provider: 'aws',
          bucket: 'my-bucket',
          created_at: '2026-01-01T00:00:00Z',
        },
      ],
      isLoading: false,
    } as unknown as ReturnType<typeof useBackupTargetsOptional>)

    renderBackupsPage()

    expect(screen.getByText('No backups yet')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Add a backup target' }),
    ).not.toBeInTheDocument()
  })

  it('does not prompt for a target while that query is still loading', () => {
    vi.mocked(useBackupTargetsOptional).mockReturnValue({
      data: undefined,
      isLoading: true,
    } as unknown as ReturnType<typeof useBackupTargetsOptional>)

    renderBackupsPage()

    expect(screen.getByText('No backups yet')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Add a backup target' }),
    ).not.toBeInTheDocument()
  })
})
