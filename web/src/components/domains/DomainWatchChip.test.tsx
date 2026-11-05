import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { secondsUntil } from '../../lib/watchTime'
import { WatchChipView } from './DomainWatchChip'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: { count?: number }) =>
      o?.count === undefined ? key : `${key}:${o.count}`,
  }),
}))

const NOW = Date.parse('2026-11-02T10:00:00Z')

describe('WatchChipView', () => {
  it.each([
    'waiting_dns',
    'propagating',
    'waiting_certificate',
    'stalled',
  ] as const)('shows the %s state', (state) => {
    render(<WatchChipView entry={{ domain: 'a.com', state }} now={NOW} />)
    expect(screen.getByRole('status')).toHaveTextContent(`watch.state.${state}`)
  })

  it('shows seconds, then minutes, until the next check', () => {
    const { rerender } = render(
      <WatchChipView
        entry={{
          domain: 'a.com',
          state: 'waiting_dns',
          next_check: '2026-11-02T10:00:12Z',
        }}
        now={NOW}
      />,
    )
    expect(screen.getByRole('status')).toHaveTextContent(
      'watch.nextInSeconds:12',
    )
    rerender(
      <WatchChipView
        entry={{
          domain: 'a.com',
          state: 'waiting_dns',
          next_check: '2026-11-02T10:02:30Z',
        }}
        now={NOW}
      />,
    )
    expect(screen.getByRole('status')).toHaveTextContent(
      'watch.nextInMinutes:3',
    )
  })

  it('omits the countdown for a stopped entry and renders nothing when live', () => {
    const { container, rerender } = render(
      <WatchChipView
        entry={{
          domain: 'a.com',
          state: 'stalled',
          stopped: true,
          next_check: '2026-11-02T10:00:12Z',
        }}
        now={NOW}
      />,
    )
    expect(screen.getByRole('status')).not.toHaveTextContent('nextIn')
    rerender(
      <WatchChipView entry={{ domain: 'a.com', state: 'live' }} now={NOW} />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe('secondsUntil', () => {
  it('is null for missing, invalid or past times', () => {
    expect(secondsUntil(undefined, NOW)).toBeNull()
    expect(secondsUntil('nope', NOW)).toBeNull()
    expect(secondsUntil('2026-11-02T09:59:00Z', NOW)).toBeNull()
  })
})
