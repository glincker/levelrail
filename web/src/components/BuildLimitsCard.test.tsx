import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../test/allI18n'
import { BuildLimitsCard } from './BuildLimitsCard'

const values = {
  max_concurrent: 2,
  timeout_seconds: 3600,
  memory_mb: 4096,
  cpu_millis: 2000,
}
let state: { isPending: boolean; data?: unknown; error: Error | null }
vi.mock('../queries/buildPerformance', () => ({
  useAppBuildLimits: () => state,
  useBuildStats: () => ({ data: undefined, isPending: true }),
  useSetAppBuildLimits: () => ({ isPending: false, mutate: vi.fn() }),
  useResetAppBuildLimits: () => ({ isPending: false, mutate: vi.fn() }),
}))
vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => false }))

function renderCard() {
  return render(
    <AllI18n>
      <BuildLimitsCard appName="web" />
    </AllI18n>,
  )
}

describe('BuildLimitsCard', () => {
  beforeEach(() => {
    state = { isPending: true, error: null }
  })

  it('shows a card placeholder while limits load', () => {
    renderCard()
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('renders the limits card once data arrives', () => {
    state = {
      isPending: false,
      error: null,
      data: { effective: values, global: values, overrides: {} },
    }
    const view = renderCard()
    expect(screen.queryByRole('status')).toBeNull()
    expect(view.container.querySelector('#build-limits')).not.toBeNull()
  })
})
