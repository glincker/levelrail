import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../test/allI18n'
import { BuildTrendCard } from './BuildTrendCard'

let state: { isPending: boolean; data?: unknown; error: Error | null }
vi.mock('../queries/buildPerformance', () => ({
  useBuildStats: () => state,
}))

function renderCard() {
  return render(
    <AllI18n>
      <BuildTrendCard appName="web" />
    </AllI18n>,
  )
}

describe('BuildTrendCard', () => {
  beforeEach(() => {
    state = { isPending: true, error: null }
  })

  it('shows a card placeholder while stats load', () => {
    renderCard()
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('renders the trend card once stats arrive', () => {
    state = {
      isPending: false,
      error: null,
      data: {
        app: 'web',
        limit: 20,
        runs: [],
        trend: {
          builds: 0,
          succeeded: 0,
          failed: 0,
          timed_out: 0,
          canceled: 0,
          skewed: 0,
          p50_seconds: 0,
          p95_seconds: 0,
          min_seconds: 0,
          max_seconds: 0,
          cache_hit_ratio: 0,
          cache_known: false,
        },
      },
    }
    renderCard()
    expect(screen.queryByRole('status')).toBeNull()
    expect(screen.getByText(/No builds recorded yet/)).toBeInTheDocument()
  })
})
