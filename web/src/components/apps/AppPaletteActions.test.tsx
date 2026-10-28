import { render, screen, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import type { AppPaletteAction } from '../../lib/paletteAction'
import { AppPaletteActions } from './AppPaletteActions'

const clear = vi.fn()
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('../../lib/paletteAction', async (orig) => ({
  ...(await orig<typeof import('../../lib/paletteAction')>()),
  useClearActionParam: () => clear,
}))
const restartMutate = vi.fn()
const stopMutate = vi.fn()
vi.mock('../../queries/apps', () => ({
  useRestartApp: () => ({ mutate: restartMutate, isPending: false }),
  useStopApp: () => ({ mutate: stopMutate, isPending: false }),
  useStartApp: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('../../queries/deploys', () => ({
  deployStatusQueryOptions: () => ({
    queryKey: ['deploys'],
    queryFn: () => [],
  }),
}))
const rollbackMutate = vi.fn()
vi.mock('../../queries/deployControl', () => ({
  useRollbackToDeploy: () => ({ mutate: rollbackMutate, isPending: false }),
}))
const targets = {
  data: [
    { deploy_id: 'd2', image: 'web:2', live: true },
    { deploy_id: 'd1', image: 'web:1', live: false },
  ] as { deploy_id: string; image: string; live: boolean }[] | undefined,
}
vi.mock('../../queries/deployGate', () => ({
  useRollbackTargets: () => targets,
}))

const app = { name: 'web', suspended: false } as AppDetail

function setup(action: AppPaletteAction | undefined, suspended = false) {
  const onDeploy = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AppPaletteActions
        app={{ ...app, suspended }}
        action={action}
        onDeploy={onDeploy}
      />
    </QueryClientProvider>,
  )
  return onDeploy
}

function confirmButton() {
  const dialog = screen.getByRole('dialog')
  const button = within(dialog)
    .getAllByRole('button')
    .find((b) => b.textContent !== 'Cancel')
  if (!button) throw new Error('no confirm button')
  return button
}

describe('AppPaletteActions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('deploy opens the deploy sheet and clears the param', () => {
    const onDeploy = setup('deploy')
    expect(onDeploy).toHaveBeenCalledWith('existing-image')
    expect(clear).toHaveBeenCalled()
  })

  it('restart waits for confirm before mutating', async () => {
    setup('restart')
    expect(restartMutate).not.toHaveBeenCalled()
    await userEvent.click(confirmButton())
    expect(restartMutate).toHaveBeenCalledWith('web', expect.anything())
    expect(clear).toHaveBeenCalled()
  })

  it('stop waits for confirm before mutating', async () => {
    setup('stop')
    expect(stopMutate).not.toHaveBeenCalled()
    await userEvent.click(confirmButton())
    expect(stopMutate).toHaveBeenCalledWith('web', expect.anything())
  })

  it('stop on an already stopped app opens no dialog', () => {
    setup('stop', true)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(clear).toHaveBeenCalled()
  })

  it('rollback targets the previous non-live release after confirm', async () => {
    setup('rollback')
    expect(rollbackMutate).not.toHaveBeenCalled()
    await userEvent.click(confirmButton())
    expect(rollbackMutate).toHaveBeenCalledWith(
      { deployId: 'd1' },
      expect.anything(),
    )
  })

  it('rollback with no previous release opens no dialog', () => {
    targets.data = [{ deploy_id: 'd2', image: 'web:2', live: true }]
    setup('rollback')
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('no action does nothing', () => {
    const onDeploy = setup(undefined)
    expect(onDeploy).not.toHaveBeenCalled()
    expect(clear).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
