import { screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FleetCapacityWidget } from './FleetCapacityWidget'
import { renderWidget } from './testKit'

const state = vi.hoisted(() => ({ q: {} }))
vi.mock('@/queries/fleetResourceUsage', () => ({
  useFleetResourceUsage: () => state.q,
}))

const GIB = 1024 ** 3

describe('FleetCapacityWidget', () => {
  it('shows an error state', () => {
    state.q = { error: new Error('x') }
    renderWidget(<FleetCapacityWidget />)
    expect(screen.getByText('Capacity is not available.')).toBeInTheDocument()
  })

  it('shows an empty state when no node reports', () => {
    state.q = { data: { nodes: [], fleet: { node_count: 1 } } }
    renderWidget(<FleetCapacityWidget />)
    expect(
      screen.getByText('No node reports capacity yet.'),
    ).toBeInTheDocument()
  })

  it('shows three meters with percentages', () => {
    state.q = {
      data: {
        nodes: [{ cpu_percent: 40 }],
        fleet: {
          memory_used_percent: 50,
          total_memory_usage_bytes: 4 * GIB,
          total_memory_bytes: 8 * GIB,
          disk_used_percent: 25,
        },
      },
    }
    renderWidget(<FleetCapacityWidget />)
    expect(screen.getAllByRole('meter')).toHaveLength(3)
    expect(screen.getByText('40%')).toBeInTheDocument()
    expect(screen.getByText('50% (4.0 GiB / 8.0 GiB)')).toBeInTheDocument()
  })
})
