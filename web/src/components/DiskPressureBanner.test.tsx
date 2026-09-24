import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DiskPressureBanner } from './DiskPressureBanner'

const useDockerHealthPoll = vi.fn()
vi.mock('../queries/systemStatus', () => ({
  useDockerHealthPoll: () => useDockerHealthPoll() as unknown,
}))
vi.mock('./CleanUpDockerDialog', () => ({
  CleanUpDockerDialog: () => <button type="button">Clean up</button>,
}))

const GB = 1024 ** 3

function status(freeGb: number) {
  return {
    data: {
      data_dir_total_bytes: 100 * GB,
      data_dir_free_bytes: freeGb * GB,
    },
  }
}

describe('DiskPressureBanner', () => {
  afterEach(() => {
    cleanup()
    useDockerHealthPoll.mockReset()
  })

  it('renders nothing while loading or healthy', () => {
    useDockerHealthPoll.mockReturnValue({ data: undefined })
    const { container, rerender } = render(<DiskPressureBanner />)
    expect(container).toBeEmptyDOMElement()
    useDockerHealthPoll.mockReturnValue(status(50))
    rerender(<DiskPressureBanner />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the warning level', () => {
    useDockerHealthPoll.mockReturnValue(status(8))
    render(<DiskPressureBanner />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Disk space is getting low',
    )
  })

  it('shows the critical level', () => {
    useDockerHealthPoll.mockReturnValue(status(2))
    render(<DiskPressureBanner />)
    expect(screen.getByRole('alert')).toHaveTextContent('Disk almost full')
  })

  it('can be dismissed with a named button', async () => {
    useDockerHealthPoll.mockReturnValue(status(2))
    render(<DiskPressureBanner />)
    await userEvent
      .setup()
      .click(screen.getByRole('button', { name: 'Dismiss for now' }))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
