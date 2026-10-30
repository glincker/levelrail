import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FleetHealthWidget } from './FleetHealthWidget'
import { renderWidget } from './testKit'

vi.mock('@tanstack/react-router', async () =>
  (await import('./testKit')).routerMock(),
)
const state = vi.hoisted(() => ({
  apps: {},
}))
vi.mock('@tanstack/react-query', () => ({ useQuery: () => state.apps }))
vi.mock('@/queries/appPages', () => ({ appsSummaryQueryOptions: () => ({}) }))
vi.mock('@/queries/nodes', () => ({
  useNodeListOptional: () => ({
    data: [
      { is_local: true, status: 'online' },
      { is_local: false, status: 'offline' },
    ],
  }),
}))

describe('FleetHealthWidget', () => {
  it('shows a skeleton while loading', () => {
    state.apps = { isLoading: true }
    const { container } = renderWidget(<FleetHealthWidget />)
    expect(container.querySelector('[data-slot="skeleton"]')).not.toBeNull()
  })

  it('shows an error state', () => {
    state.apps = { error: new Error('x') }
    renderWidget(<FleetHealthWidget />)
    expect(screen.getByText('Could not load this data.')).toBeInTheDocument()
  })

  it('shows an empty state with no apps', () => {
    state.apps = { data: { total: 0 } }
    renderWidget(<FleetHealthWidget />)
    expect(screen.getByText('No apps yet.')).toBeInTheDocument()
  })

  it('shows counts and nodes online', () => {
    state.apps = { data: { total: 5, running: 3, failing: 1, stopped: 1 } }
    renderWidget(<FleetHealthWidget />)
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('1 / 2')).toBeInTheDocument()
    expect(screen.getByRole('img').getAttribute('aria-label')).toContain(
      '3 running',
    )
  })
})
