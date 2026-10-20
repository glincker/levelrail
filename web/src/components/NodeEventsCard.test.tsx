import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NodeEventsCard } from './NodeEventsCard'

const useNodeEvents = vi.fn()
vi.mock('../queries/nodes', () => ({
  useNodeEvents: (id: string) => useNodeEvents(id) as unknown,
}))

describe('NodeEventsCard', () => {
  afterEach(() => {
    cleanup()
    useNodeEvents.mockReset()
  })

  it('shows an accessible loading state', () => {
    useNodeEvents.mockReturnValue({ isPending: true, isError: false })
    render(<NodeEventsCard nodeId="n1" />)
    expect(
      screen.getByRole('status', { name: 'Loading connection history' }),
    ).toBeInTheDocument()
  })

  it('announces an error', () => {
    useNodeEvents.mockReturnValue({ isPending: false, isError: true })
    render(<NodeEventsCard nodeId="n1" />)
    expect(screen.getByRole('alert')).toHaveTextContent('unavailable')
  })

  it('shows the empty state', () => {
    useNodeEvents.mockReturnValue({
      isPending: false,
      isError: false,
      data: [],
    })
    render(<NodeEventsCard nodeId="n1" />)
    expect(screen.getByText('No status changes recorded yet.')).toBeVisible()
  })

  it('lists transitions', () => {
    useNodeEvents.mockReturnValue({
      isPending: false,
      isError: false,
      data: [
        {
          created_at: '2026-09-22T10:00:00Z',
          from_status: 'online',
          to_status: 'offline',
        },
        {
          created_at: '2026-09-22T10:00:00Z',
          from_status: 'offline',
          to_status: 'online',
        },
      ],
    })
    render(<NodeEventsCard nodeId="n1" />)
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(useNodeEvents).toHaveBeenCalledWith('n1')
  })
})
