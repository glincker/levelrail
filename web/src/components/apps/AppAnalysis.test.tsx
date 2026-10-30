import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { latestReport, sortCauses } from '@/lib/analysisReports'
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
  it('picks the latest report', () => {
    expect(
      latestReport([
        rep('a', '2026-01-01T00:00:00Z'),
        rep('b', '2026-02-01T00:00:00Z'),
      ])?.id,
    ).toBe('b')
    expect(latestReport([])).toBeUndefined()
  })
})

describe('AppAnalysisView', () => {
  it('renders nothing when empty', () => {
    const { container } = render(<AppAnalysisView reports={[]} />)
    expect(container.firstChild).toBeNull()
  })
  it('renders the latest summary with a toggle', () => {
    render(<AppAnalysisView reports={[rep('a', '2026-01-01T00:00:00Z')]} />)
    expect(screen.getByText('summary a')).toBeTruthy()
    expect(screen.getByRole('button').getAttribute('aria-expanded')).toBe(
      'false',
    )
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
