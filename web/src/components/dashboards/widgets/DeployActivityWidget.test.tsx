import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DeployActivityWidget } from './DeployActivityWidget'
import { renderWidget } from './testKit'

vi.mock('@tanstack/react-router', async () =>
  (await import('./testKit')).routerMock(),
)
const state = vi.hoisted(() => ({
  apps: {},
  deploys: {},
}))
vi.mock('@/queries/apps', () => ({ useAppListOptional: () => state.apps }))
vi.mock('@/queries/fleetTraffic', () => ({
  useFleetDeploys: () => state.deploys,
}))
vi.mock('@/hooks/useMinuteNow', () => ({ useMinuteNow: () => Date.now() }))
vi.mock('@/components/dashboard/FleetTiles', () => ({ SAMPLE_APP_CAP: 8 }))

const app = { name: 'web', status: { variant: 'success' } }
const attempt = (id: string, status: string) => ({
  id,
  status,
  started_at: new Date().toISOString(),
})

describe('DeployActivityWidget', () => {
  it('shows an error state', () => {
    state.apps = { error: new Error('x') }
    state.deploys = { byApp: {}, isPending: false }
    renderWidget(<DeployActivityWidget />)
    expect(screen.getByText('Could not load this data.')).toBeInTheDocument()
  })

  it('shows an empty state', () => {
    state.apps = { data: [app] }
    state.deploys = { byApp: { web: [] }, isPending: false }
    renderWidget(<DeployActivityWidget />)
    expect(screen.getByText('No finished deploys yet.')).toBeInTheDocument()
  })

  it('summarises outcomes and lists the latest', () => {
    state.apps = { data: [app] }
    state.deploys = {
      byApp: { web: [attempt('1', 'succeeded'), attempt('2', 'failed')] },
      isPending: false,
    }
    renderWidget(<DeployActivityWidget />)
    expect(
      screen.getByText('2 in 24 hours, 2 in 7 days (1 succeeded, 1 failed)'),
    ).toBeInTheDocument()
    expect(screen.getAllByRole('link')).toHaveLength(2)
  })
})
