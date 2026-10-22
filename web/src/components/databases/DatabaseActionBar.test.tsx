import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import type { DatabaseResource } from '../../types/databaseDetail'
import { DatabaseActionBar } from './DatabaseActionBar'

const backupMutate = vi.fn()
const deleteMutate = vi.fn()

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})
vi.mock('../../queries/backupHistory', () => ({
  useTriggerBackup: () => ({ mutate: backupMutate, isPending: false }),
}))
vi.mock('../../queries/databases', () => ({
  useStartDatabase: () => ({ mutate: vi.fn(), mutateAsync: vi.fn() }),
  useStopDatabase: () => ({ mutate: vi.fn(), mutateAsync: vi.fn() }),
  useDeleteDatabase: () => ({
    mutate: deleteMutate,
    reset: vi.fn(),
    isPending: false,
    isError: false,
    error: null,
  }),
}))

const base = {
  name: 'orders-db',
  engine: 'postgres',
  version: '16',
  backup_target_id: 'tgt-1',
} as DatabaseResource

function renderBar(db: DatabaseResource) {
  return render(
    <DatabasesI18n>
      <DatabaseActionBar database={db} />
    </DatabasesI18n>,
  )
}

describe('DatabaseActionBar', () => {
  it('leads with Back up now and keeps Delete out of the header', async () => {
    renderBar(base)
    await userEvent.click(screen.getByRole('button', { name: 'Back up now' }))
    expect(backupMutate).toHaveBeenCalledWith('tgt-1', expect.anything())
    expect(screen.queryByRole('button', { name: /delete/i })).toBeNull()
  })

  it('offers backup setup when no target is configured', () => {
    renderBar({ ...base, backup_target_id: undefined })
    expect(
      screen.getByRole('button', { name: 'Set up backups' }),
    ).toBeInTheDocument()
  })

  it('leads with Start when the database is stopped', () => {
    renderBar({ ...base, suspended: true })
    expect(screen.getByRole('button', { name: 'Start' })).toBeInTheDocument()
  })

  it('requires typing the name before deleting, without window.confirm', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm')
    renderBar(base)
    await userEvent.click(screen.getByRole('button', { name: 'More actions' }))
    await userEvent.click(await screen.findByText('Delete...'))
    const confirm = await screen.findByRole('button', {
      name: 'Delete database',
    })
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/to confirm/), 'orders-db')
    expect(confirm).toBeEnabled()
    await userEvent.click(confirm)
    expect(deleteMutate).toHaveBeenCalledWith(
      { name: 'orders-db', force: false, deleteData: false },
      expect.anything(),
    )
    expect(confirmSpy).not.toHaveBeenCalled()
  })
})
