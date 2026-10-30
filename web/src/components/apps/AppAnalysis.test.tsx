import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { sortCauses, sortReportsNewest } from '@/lib/analysisReports'
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
