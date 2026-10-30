import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  formatSizingLimit,
  sortCauses,
  sortReportsNewest,
  summarizeLogGroups,
  usableMetrics,
} from '@/lib/analysisReports'
import type {
  AnalysisCause,
  AnalysisReportSummary,
} from '../../types/analysisReport'
import { AnalysisReportBodyView, AppAnalysisView } from './AppAnalysis'

const cause = (
  id: string,
  confidence: string,
  score: number,
): AnalysisCause => ({
  rank: 1,
  id,
  title: id,
  score,
  confidence,
  evidence: ['m1'],
})
const rep = (id: string, created_at: string): AnalysisReportSummary => ({
  id,
  app: 'web',
  trigger_kind: 'alert',
  trigger_id: 't',
  status: 'complete',
  summary: `summary ${id}`,
  created_at,
})

describe('helpers', () => {
  it('sorts causes by confidence then score', () => {
    const out = sortCauses([
      cause('a', 'low', 90),
      cause('b', 'high', 10),
      cause('c', 'high', 50),
    ])
    expect(out.map((c) => c.id)).toEqual(['c', 'b', 'a'])
  })
  it('sorts reports newest first', () => {
    const out = sortReportsNewest([
      rep('a', '2026-01-01T00:00:00Z'),
      rep('b', '2026-02-01T00:00:00Z'),
    ])
    expect(out.map((r) => r.id)).toEqual(['b', 'a'])
  })
})

describe('AppAnalysisView', () => {
  it('renders nothing when empty', () => {
    const { container } = render(<AppAnalysisView reports={[]} />)
    expect(container.firstChild).toBeNull()
  })
  it('renders every report with its own toggle', () => {
    render(
      <AppAnalysisView
        reports={[
          rep('a', '2026-01-01T00:00:00Z'),
          rep('b', '2026-02-01T00:00:00Z'),
        ]}
      />,
    )
    expect(screen.getByText('summary a')).toBeTruthy()
    expect(screen.getByText('summary b')).toBeTruthy()
    const buttons = screen.getAllByRole('button')
    expect(buttons).toHaveLength(2)
    expect(buttons[0]?.getAttribute('aria-expanded')).toBe('false')
  })
})

describe('AnalysisReportBodyView', () => {
  it('shows evidence ids as plain text', () => {
    render(
      <AnalysisReportBodyView
        body={{
          status: 'complete',
          summary: 's',
          timeline: null,
          causes: [cause('<b>x</b>', 'high', 1)],
          recommendations: null,
        }}
      />,
    )
    expect(screen.getByText('<b>x</b>')).toBeTruthy()
    expect(screen.getByText('m1')).toBeTruthy()
  })
})

describe('digest evidence', () => {
  const base = {
    status: 'complete' as const,
    summary: 's',
    timeline: null,
    causes: null,
    recommendations: null,
  }
  const metric = {
    evidence: 'metric:mem',
    metric: 'mem',
    samples: 5,
    min: 1,
    p50: 2,
    p95: 3,
    max: 4,
    mean: 2,
    baseline_mean: 1,
    trend_pct: 10,
    trend: 'rising',
    anomaly_count: 0,
    max_z: 0,
  }
  it('renders nothing for an empty digest', () => {
    render(
      <AnalysisReportBodyView
        body={{
          ...base,
          digest: { metrics: [], logs: [], log_lines: 0, restarts: 0 },
        }}
      />,
    )
    expect(
      screen.queryByRole('region', { name: 'appAnalysis.evidence.title' }),
    ).toBeNull()
  })
  it('shows metrics, sizing and log counts', () => {
    render(
      <AnalysisReportBodyView
        body={{
          ...base,
          digest: {
            metrics: [
              metric,
              { ...metric, evidence: 'metric:x', metric: 'x', samples: 0 },
            ],
            logs: [
              {
                evidence: 'log:error:a',
                level: 'error',
                fingerprint: 'f',
                sample: 's',
                count: 7,
                first_seen: '2026-01-01T00:00:00Z',
                last_seen: '2026-01-01T00:00:00Z',
              },
            ],
            log_lines: 7,
            restarts: 0,
            sizing: [
              {
                dimension: 'memory',
                action: 'lower',
                confidence: 'high',
                data_sufficient: true,
                current_limit: 512 * 1024 * 1024,
                p95_usage: 1,
                suggested_limit: 256 * 1024 * 1024,
                suggested_text: '256 MiB',
              },
            ],
          },
        }}
      />,
    )
    expect(screen.getByText('mem')).toBeTruthy()
    expect(screen.queryByText('x')).toBeNull()
    expect(screen.getByText(/appAnalysis.evidence.sizingChange/)).toBeTruthy()
  })
  it('helpers', () => {
    expect(usableMetrics(null)).toEqual([])
    expect(formatSizingLimit('cpu', 1.5e9)).toBe('1.5 cores')
    expect(formatSizingLimit('memory', 0)).toBe('')
    expect(
      summarizeLogGroups([
        {
          evidence: 'a',
          level: 'warn',
          fingerprint: '',
          sample: '',
          count: 2,
          first_seen: '',
          last_seen: '',
        },
        {
          evidence: 'b',
          level: 'warn',
          fingerprint: '',
          sample: '',
          count: 3,
          first_seen: '',
          last_seen: '',
        },
      ]),
    ).toEqual([{ level: 'warn', lines: 5, groups: 2 }])
  })
})
