import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { DeployEvent } from '../../types/deployEvents'
import { EventList } from './EventList'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: { defaultValue?: string; count?: number }) =>
      o?.defaultValue ??
      (o?.count === undefined ? key : `${key}:${String(o.count)}`),
  }),
}))
vi.mock('@/components/kit', () => ({
  StatusPill: ({ label }: { label: string }) => <span>{label}</span>,
}))

function ev(id: string, over: Partial<DeployEvent>): DeployEvent {
  return {
    id,
    at: '2026-10-01T12:00:00Z',
    kind: 'step',
    phase: 'deploy',
    status: 'ok',
    code: `step.${id}`,
    title: `title ${id}`,
    ...over,
  }
}

const events: DeployEvent[] = [
  ev('trigger', {
    kind: 'trigger',
    phase: 'trigger',
    status: 'info',
    title: 'Git push received',
    detail: 'main abc1234',
  }),
  ev('l1', { kind: 'log', stream: 'stdout', detail: 'step 1/3' }),
  ev('l2', { kind: 'log', stream: 'stderr', detail: 'warn: slow layer' }),
  ev('watch', {
    kind: 'watch',
    phase: 'watch',
    status: 'failed',
    code: 'watch.crashloop',
    title: 'Crashloop after deploy',
    reason: 'PostDeployCrashloop',
  }),
  ev('impact', {
    kind: 'traffic_impact',
    phase: 'watch',
    status: 'info',
    code: 'traffic.impact',
    title: 'Traffic impact',
    impact: {
      cutover_at: '2026-10-01T12:00:00Z',
      window_ms: 300_000,
      complete: true,
      before: {
        has_traffic: true,
        requests: 100,
        rate_per_sec: 10,
        error_rate_5xx: 0.01,
        p95_ms: 80,
      },
      after: {
        has_traffic: true,
        requests: 90,
        rate_per_sec: 9,
        error_rate_5xx: 0.2,
        p95_ms: 400,
      },
    },
  }),
]

describe('EventList', () => {
  it('renders events in order with details and reasons', () => {
    render(<EventList events={events} />)
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(4)
    expect(screen.getByText('main abc1234')).toBeTruthy()
    expect(screen.getByText('PostDeployCrashloop')).toBeTruthy()
    expect(screen.getByText('20%')).toBeTruthy()
  })

  it('folds build output into a collapsed, keyboard reachable group', () => {
    const { container } = render(<EventList events={events} />)
    const details = container.querySelector('details')
    expect(details).not.toBeNull()
    expect(details?.open).toBe(false)
    const summary = screen.getByText('logs.summary:2')
    expect(summary.tagName).toBe('SUMMARY')
    fireEvent.click(summary)
    expect(details?.open).toBe(true)
    const region = screen.getByLabelText('logs.region')
    expect(region.getAttribute('tabindex')).toBe('0')
    expect(region.textContent).toContain('warn: slow layer')
  })

  it('hides build output entirely when asked', () => {
    const { container } = render(<EventList events={events} showLogs={false} />)
    expect(container.querySelector('details')).toBeNull()
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
  })

  it('does not rely on color alone: failed rows carry a text status', () => {
    render(<EventList events={events} />)
    expect(screen.getAllByText('status.failed').length).toBeGreaterThan(0)
  })
})
