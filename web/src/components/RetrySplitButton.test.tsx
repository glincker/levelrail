import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../test/commonI18n'
import { RetrySplitButton } from './RetrySplitButton'

function setup(canRebuild: boolean) {
  const onRetry = vi.fn()
  render(
    <CommonI18n>
      <RetrySplitButton
        onRetry={onRetry}
        onRebuild={vi.fn()}
        onRebuildNoCache={vi.fn()}
        canRebuild={canRebuild}
      />
    </CommonI18n>,
  )
  return { onRetry }
}

describe('RetrySplitButton', () => {
  it('retries with the primary button', async () => {
    const { onRetry } = setup(true)
    await userEvent.click(screen.getByRole('button', { name: /retry deploy/i }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('hides the rebuild menu when the app has no source to build', () => {
    setup(false)
    expect(
      screen.queryByRole('button', { name: /more ways to retry/i }),
    ).toBeNull()
  })

  it('offers the rebuild menu for built apps', () => {
    setup(true)
    expect(
      screen.getByRole('button', { name: /more ways to retry/i }),
    ).toBeTruthy()
  })
})
