import type { ReactElement, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import observabilityEn from '../locales/en/observability.json'
import type { DeployAttempt } from '../types/deployAttempt'
import type { FailureContext } from '../types/investigate'
import { ChartMarkerDot } from './ChartMarkerDot'
import { DeployMarkerDetails } from './DeployMarkerDetails'
import { WhatChangedTimeline } from './WhatChangedTimeline'
import { FailureContextCard } from './FailureContextCard'
import { bucketRestarts, deployMarkers } from '../lib/chartMarkers'
import type { ChartMarker } from '../lib/metricChart'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

let failure: FailureContext = {
  state: 'healthy',
  restarts_in_window: 0,
  window_seconds: 900,
  lines: [],
  total_lines: 0,
}
vi.mock('../queries/investigate', () => ({
  useFailureContext: () => ({ data: failure }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['observability'],
  defaultNS: 'observability',
  resources: { en: { observability: observabilityEn } },
  interpolation: { escapeValue: false },
})

function renderObs(ui: ReactElement) {
  return render(<I18nextProvider i18n={testI18n}>{ui}</I18nextProvider>)
}

const attempt: DeployAttempt = {
  id: 'dpl_1',
  service_name: 'web',
  image: 'registry/web:v2',
  commit_sha: 'abcdef1234567',
  status: 'failed',
  started_at: '2026-10-10T03:00:00Z',
  finished_at: '2026-10-10T03:01:30Z',
  error: 'readiness probe failed',
}

const marker: ChartMarker = {
  key: 'dpl_1',
  t: 1,
  color: 'red',
  tooltip: 'Failed deploy: registry/web:v2',
  kind: 'deploy',
}

describe('deploy marker', () => {
  it('is a button that reports the clicked marker, by mouse and keyboard', () => {
    const onClick = vi.fn()
    render(
      <svg>
        <ChartMarkerDot marker={marker} x={10} y={10} onClick={onClick} />
      </svg>,
    )
    const button = screen.getByRole('button', { name: marker.tooltip })
    fireEvent.click(button)
    fireEvent.keyDown(button, { key: 'Enter' })
    expect(onClick).toHaveBeenCalledTimes(2)
    expect(onClick).toHaveBeenCalledWith(marker)
  })

  it('is not interactive without a handler', () => {
    render(
      <svg>
        <ChartMarkerDot marker={marker} x={10} y={10} />
      </svg>,
    )
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('details dialog shows status, short commit, duration and logs link', () => {
    renderObs(
      <DeployMarkerDetails appName="web" attempt={attempt} onClose={vi.fn()} />,
    )
    expect(screen.getByText('Failed deploy')).toBeInTheDocument()
    expect(screen.getByText('abcdef1')).toBeInTheDocument()
    expect(screen.getByText('1m 30s')).toBeInTheDocument()
    expect(screen.getByText('readiness probe failed')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Open deploy logs' }),
    ).toBeInTheDocument()
  })

  it('renders nothing when no deploy is selected', () => {
    renderObs(
      <DeployMarkerDetails appName="web" attempt={null} onClose={vi.fn()} />,
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })
})

describe('chart markers', () => {
  const range = {
    from: new Date('2026-10-10T00:00:00Z'),
    to: new Date('2026-10-10T06:00:00Z'),
  }

  it('keeps only deploys inside the range, oldest first', () => {
    const attempts = [
      { ...attempt, id: 'late', started_at: '2026-10-10T05:00:00Z' },
      { ...attempt, id: 'out', started_at: '2026-10-09T05:00:00Z' },
      { ...attempt, id: 'early', started_at: '2026-10-10T01:00:00Z' },
    ]
    const out = deployMarkers(attempts, range, (a) => a.id)
    expect(out.map((m) => m.key)).toEqual(['early', 'late'])
  })

  it('collapses a restart burst into one bucket with a count', () => {
    const at = (m: number) => ({
      timestamp: new Date(range.from.getTime() + m * 60_000).toISOString(),
      value: 1,
      count: 1,
    })
    const buckets = bucketRestarts([at(10), at(11), at(12), at(300)], range)
    expect(buckets.map((b) => b.count)).toEqual([3, 1])
  })
})

describe('WhatChangedTimeline', () => {
  it('renders oldest first and flags the likely cause', () => {
    renderObs(
      <WhatChangedTimeline
        events={[
          {
            at: '2026-10-10T03:10:00Z',
            kind: 'restart',
            severity: 'warning',
            title: 'Container restarted',
          },
          {
            at: '2026-10-10T03:00:00Z',
            kind: 'deploy',
            severity: 'info',
            title: 'Deploy v2',
            likely_cause: true,
          },
        ]}
      />,
    )
    const items = screen.getAllByRole('listitem')
    expect(items[0]).toHaveTextContent('Deploy v2')
    expect(items[0]).toHaveTextContent('Likely cause')
    expect(items[1]).toHaveTextContent('Container restarted')
  })

  it('says so when nothing changed', () => {
    renderObs(<WhatChangedTimeline events={[]} />)
    expect(
      screen.getByText('Nothing changed around this time.'),
    ).toBeInTheDocument()
  })
})

describe('FailureContextCard', () => {
  it('renders nothing for a healthy app', () => {
    failure = { ...failure, state: 'healthy' }
    const { container } = renderObs(<FailureContextCard appName="web" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the last lines of a crashlooping container', () => {
    failure = {
      state: 'crashlooping',
      restarts_in_window: 7,
      window_seconds: 900,
      lines: [
        {
          timestamp: '2026-10-10T03:00:00Z',
          stream: 'stderr',
          message: 'panic: boom',
        },
      ],
      total_lines: 200,
    }
    renderObs(<FailureContextCard appName="web" />)
    expect(screen.getByText('This app is crashlooping')).toBeInTheDocument()
    expect(screen.getByText('panic: boom')).toBeInTheDocument()
    expect(
      screen.getByText(/7 restarts in the last 15 minutes/),
    ).toBeInTheDocument()
  })
})
