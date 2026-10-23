import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DeployDetailI18n } from '../../test/deployDetailI18n'
import { StreamDropBanner } from './StreamDropBanner'

function renderBanner(retryAt: number | null) {
  render(
    <DeployDetailI18n>
      <StreamDropBanner
        transport="sse"
        connectionState="error"
        retryAt={retryAt}
        onRetry={() => undefined}
        onSwitchToPolling={() => undefined}
        downloadUrl="/log"
      />
    </DeployDetailI18n>,
  )
}

describe('StreamDropBanner', () => {
  it('keeps the ticking countdown out of the live region', () => {
    renderBanner(Date.now() + 30_000)
    const live = screen.getByRole('status')
    expect(live).toHaveTextContent('The live log stream dropped')
    expect(live).not.toHaveTextContent(/Next attempt/)
    const countdown = screen.getByText(/Next attempt in \d+s/)
    expect(live.contains(countdown)).toBe(false)
    expect(countdown).toHaveAttribute('aria-hidden', 'true')
  })

  it('omits the countdown when no retry is scheduled', () => {
    renderBanner(null)
    expect(screen.queryByText(/Next attempt/)).toBeNull()
  })
})
