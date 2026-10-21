import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DockerDegradedBanner } from './DockerDegradedBanner'
import { useDockerDegraded } from './useDockerDegraded'

const poll = vi.fn()
vi.mock('../../queries/dockerIntegration', () => ({
  useDockerDegradedPoll: () => poll() as unknown,
}))
vi.mock('@tanstack/react-router', () => ({
  Link: () => <a href="/images">link</a>,
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

function Harness() {
  const state = useDockerDegraded()
  return <DockerDegradedBanner state={state} />
}

const down = {
  data: { unresponsive: true, unresponsive_since: '2026-10-21T10:00:00Z' },
  isFetching: false,
  refetch: vi.fn(),
}

describe('DockerDegradedBanner', () => {
  afterEach(() => {
    cleanup()
    poll.mockReset()
  })

  it('renders nothing while Docker is healthy', () => {
    poll.mockReturnValue({
      data: { unresponsive: false },
      isFetching: false,
      refetch: vi.fn(),
    })
    const { container } = render(<Harness />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows when unresponsive, rechecks, and dismisses', async () => {
    poll.mockReturnValue(down)
    render(<Harness />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'dockerDegraded.message',
    )
    const user = userEvent.setup()
    await user.click(
      screen.getByRole('button', { name: 'dockerDegraded.recheck' }),
    )
    expect(down.refetch).toHaveBeenCalled()
    await user.click(
      screen.getByRole('button', { name: 'dockerDegraded.dismiss' }),
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
