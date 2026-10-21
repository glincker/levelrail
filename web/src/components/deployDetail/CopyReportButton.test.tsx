import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DeployDetailI18n } from '../../test/deployDetailI18n'
import { CopyReportButton } from './CopyReportButton'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const getInput = () => ({
  appName: 'web',
  deployId: 'd1',
  stageLabel: 'Build',
  error: 'boom',
  lines: [],
  config: {},
})

function setup() {
  return render(
    <DeployDetailI18n>
      <CopyReportButton getInput={getInput} />
    </DeployDetailI18n>,
  )
}

describe('CopyReportButton', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
  })
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
  })

  it('announces the copy through a status region only after copying', async () => {
    setup()
    expect(screen.queryByRole('status')).toBeNull()
    await act(async () => {
      screen.getByRole('button').click()
      await Promise.resolve()
    })
    expect(screen.getByRole('status')).toHaveTextContent('Report copied')
    await act(async () => {
      vi.advanceTimersByTime(1600)
      await Promise.resolve()
    })
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('clears its reset timer on unmount', async () => {
    const { unmount } = setup()
    await act(async () => {
      screen.getByRole('button').click()
      await Promise.resolve()
    })
    const clear = vi.spyOn(window, 'clearTimeout')
    unmount()
    expect(clear).toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })
})
