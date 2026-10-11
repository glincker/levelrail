import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import type { DatabaseResource } from '../types/databaseDetail'
import { MajorUpgradeCard } from './MajorUpgradeCard'

const rollbackMutate = vi.fn()

vi.mock('../queries/pitr', () => ({
  usePITRStatus: () => ({ data: { enabled: false } }),
}))
vi.mock('../queries/majorUpgrades', () => ({
  useMajorUpgrades: () => ({
    data: [
      {
        id: 'u1',
        database_name: 'orders-db',
        from_version: '15',
        to_version: '16',
        status: 'succeeded',
        snapshot_volume: 'vol-1',
        started_at: '2026-10-01T00:00:00Z',
      },
    ],
  }),
  useStartMajorUpgrade: () => ({ mutate: vi.fn(), isPending: false }),
  useRollbackMajorUpgrade: () => ({
    mutate: rollbackMutate,
    isPending: false,
    error: null,
  }),
  useDiscardMajorUpgradeSnapshot: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
  }),
}))

describe('MajorUpgradeCard', () => {
  it('asks for rollback in a dialog, never window.confirm', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm')
    render(
      <DatabasesI18n>
        <MajorUpgradeCard
          database={
            { name: 'orders-db', engine: 'postgres' } as DatabaseResource
          }
        />
      </DatabasesI18n>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Roll back' }))
    expect(rollbackMutate).not.toHaveBeenCalled()
    expect(await screen.findByText('Roll back to 15?')).toBeInTheDocument()
    const buttons = screen.getAllByRole('button', { name: 'Roll back' })
    await userEvent.click(buttons[buttons.length - 1] as HTMLElement)
    expect(rollbackMutate).toHaveBeenCalledWith('u1', expect.anything())
    expect(confirmSpy).not.toHaveBeenCalled()
  })
})
