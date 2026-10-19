import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { DatabaseResource } from '../../types/databaseDetail'
import { DatabasePaletteActions } from './DatabasePaletteActions'

const clear = vi.fn()
vi.mock('../../lib/paletteAction', async (orig) => ({
  ...(await orig<typeof import('../../lib/paletteAction')>()),
  useClearActionParam: () => clear,
}))
const backupMutate = vi.fn()
vi.mock('../../queries/backupHistory', () => ({
  useTriggerBackup: () => ({ mutate: backupMutate, isPending: false }),
}))
vi.mock('@/lib/databaseCatalog', () => ({ engineHasBackups: () => true }))

const db = {
  name: 'main',
  engine: 'postgres',
  suspended: false,
  backup_target_id: 'bt1',
} as DatabaseResource

function confirmButton() {
  const dialog = screen.getByRole('dialog')
  const button = within(dialog)
    .getAllByRole('button')
    .find((b) => b.textContent !== 'Cancel')
  if (!button) throw new Error('no confirm button')
  return button
}

describe('DatabasePaletteActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('backup waits for confirm before mutating', async () => {
    render(<DatabasePaletteActions database={db} action="backup" />)
    expect(backupMutate).not.toHaveBeenCalled()
    await userEvent.click(confirmButton())
    expect(backupMutate).toHaveBeenCalledWith('bt1', expect.anything())
    expect(clear).toHaveBeenCalled()
  })

  it('opens nothing without a backup target', () => {
    render(
      <DatabasePaletteActions
        database={{ ...db, backup_target_id: undefined }}
        action="backup"
      />,
    )
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('ignores a missing action', () => {
    render(<DatabasePaletteActions database={db} action={undefined} />)
    expect(clear).not.toHaveBeenCalled()
  })
})
