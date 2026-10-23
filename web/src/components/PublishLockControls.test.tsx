import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PublishLock } from '../queries/publishLock'
import { PublishLockBanner, PublishLockToggle } from './PublishLockControls'

const lockMutate = vi.fn()
const unlockMutate = vi.fn()
const state: { data: PublishLock | undefined } = { data: undefined }

vi.mock('../queries/publishLock', () => ({
  usePublishLock: () => ({ data: state.data }),
  usePublishLockActions: () => ({
    lock: { mutate: lockMutate, isPending: false },
    unlock: { mutate: unlockMutate, isPending: false },
  }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) =>
      typeof opts?.count === 'number' ? `${key}:${opts.count}` : key,
  }),
}))

describe('PublishLockControls', () => {
  beforeEach(() => {
    lockMutate.mockReset()
    unlockMutate.mockReset()
  })

  it('renders nothing while the lock state is unknown', () => {
    state.data = undefined
    const { container } = render(<PublishLockToggle appName="web" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('locks an unlocked app from the header toggle', async () => {
    state.data = { locked: false, held_builds: [] }
    render(<PublishLockToggle appName="web" />)
    await userEvent.click(
      screen.getByRole('button', { name: 'publishLock.lock' }),
    )
    expect(lockMutate).toHaveBeenCalledWith('', expect.anything())
  })

  it('shows who locked it and the held count, and unlocks from the banner', async () => {
    state.data = {
      locked: true,
      locked_by: 'gagan',
      reason: 'launch week',
      held_builds: ['d1', 'd2'],
    }
    render(<PublishLockBanner appName="web" />)
    expect(screen.getByText(/publishLock.bannerBody:2/)).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'publishLock.unlock' }),
    )
    expect(unlockMutate).toHaveBeenCalled()
  })

  it('hides the banner when unlocked', () => {
    state.data = { locked: false, held_builds: ['d1'] }
    const { container } = render(<PublishLockBanner appName="web" />)
    expect(container).toBeEmptyDOMElement()
  })
})
