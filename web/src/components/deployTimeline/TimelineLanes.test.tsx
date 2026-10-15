import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { Lane, TimelineDeploy } from '../../lib/deployTimeline'
import { TimelineLanes } from './TimelineLanes'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: Record<string, unknown>) => {
    const domProps = Object.fromEntries(
      Object.entries(props).filter(
        ([k]) =>
          k.startsWith('aria-') ||
          k.startsWith('data-') ||
          k.startsWith('on') ||
          k === 'className',
      ),
    )
    return <a href="#x" {...domProps} />
  },
}))
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 40,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        start: index * 40,
      })),
  }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: Record<string, unknown>) =>
      o ? `${key} ${JSON.stringify(o)}` : key,
  }),
}))

const WS = Date.parse('2026-10-14T00:00:00Z')
const END = WS + 24 * 3_600_000

function d(id: string, minute: number, secs: number): TimelineDeploy {
  const s = WS + minute * 60_000
  return {
    id,
    app: 'static-site-api-staging-long-name',
    status: 'ready',
    trigger: '',
    environment: '',
    commit_sha: 'abcdef1234',
    commit_message: 'msg',
    author: 'dev',
    strategy: 'rolling',
    project_id: '',
    node_id: '',
    started_at: new Date(s).toISOString(),
    finished_at: new Date(s + secs * 1000).toISOString(),
    duration_ms: secs * 1000,
    rollback_of: null,
    is_live: false,
  }
}

function lanes(deploys: TimelineDeploy[]): Lane[] {
  return [{ app: deploys[0]?.app ?? '', deploys }]
}

describe('TimelineLanes', () => {
  it('renders a short deploy as a focusable dot with a full-name tooltip', () => {
    const { container } = render(
      <TimelineLanes
        lanes={lanes([d('a', 300, 20)])}
        alerts={[]}
        zoom="24h"
        windowStart={WS}
        windowEnd={END}
      />,
    )
    expect(container.querySelector('[data-marker="dot"]')).not.toBeNull()
    expect(
      container.querySelector('[title="static-site-api-staging-long-name"]'),
    ).not.toBeNull()
  })

  it('groups nearby deploys into a chip that expands, and Escape closes the card', () => {
    render(
      <TimelineLanes
        lanes={lanes([d('a', 300, 20), d('b', 303, 20), d('c', 306, 20)])}
        alerts={[]}
        zoom="24h"
        windowStart={WS}
        windowEnd={END}
      />,
    )
    const chip = screen.getByRole('button', { name: /chart.clusterLabel/ })
    expect(screen.queryAllByRole('link')).toHaveLength(0)
    fireEvent.click(chip)
    const links = screen.getAllByRole('link')
    expect(links).toHaveLength(3)
    fireEvent.mouseEnter(links[0] as HTMLElement)
    expect(screen.queryByTestId('timeline-card')).not.toBeNull()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(screen.queryByTestId('timeline-card')).toBeNull()
  })
})
