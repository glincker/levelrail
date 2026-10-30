import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { TimelineEntry } from '../../queries/appTimeline'
import { AppHistoryTimeline } from './AppHistoryTimeline'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => (
    <a href="#x">{children}</a>
  ),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: Record<string, unknown>) =>
      o ? `${key} ${JSON.stringify(o)}` : key,
  }),
}))

const attempt: DeployAttempt = {
  id: 'a1',
  service_name: 'web',
  image: 'img:1',
  status: 'succeeded',
  started_at: '2026-10-25T10:00:00Z',
  finished_at: '2026-10-25T10:01:30Z',
  commit_sha: 'abcdef1234567',
  author: 'ana',
}

const restart: TimelineEntry = {
  id: 't1',
  at: '2026-10-25T11:00:00Z',
  kind: 'restart',
  status: 'failed',
  actor: 'bob',
  title: 'Restarted',
}

describe('AppHistoryTimeline', () => {
  it('shows who, short sha, status and duration for each entry', () => {
    render(
      <AppHistoryTimeline
        appName="web"
        attempts={[attempt]}
        activity={[restart]}
      />,
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('ana')).toBeTruthy()
    expect(screen.getByText('abcdef1')).toBeTruthy()
    expect(screen.getByText('1m 30s')).toBeTruthy()
    expect(screen.getByText('bob')).toBeTruthy()
    expect(screen.getByText('appHistory.status.failed')).toBeTruthy()
  })

  it('names the managing stack', () => {
    render(
      <AppHistoryTimeline
        appName="web"
        attempts={[attempt]}
        activity={[]}
        managedByStack="prod"
      />,
    )
    expect(screen.getByText(/appHistory.managedBy/)).toBeTruthy()
  })

  it('renders nothing without history', () => {
    const { container } = render(
      <AppHistoryTimeline appName="web" attempts={[]} activity={[]} />,
    )
    expect(container.firstChild).toBeNull()
  })
})
