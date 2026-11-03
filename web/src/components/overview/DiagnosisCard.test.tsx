import type { ReactNode } from 'react'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import fixitEn from '../../locales/en/fixit.json'
import type { FailureContext } from '../../types/investigate'
import type { StuckReport } from '../../queries/stuck'
import { DiagnosisCard } from './DiagnosisCard'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('../StuckPanel', () => ({
  ActionButton: ({ action }: { action: { label: string } }) => (
    <button type="button">{action.label}</button>
  ),
  CauseCard: ({ cause }: { cause: { title: string } }) => (
    <div>{cause.title}</div>
  ),
}))

let failure: FailureContext = {
  state: 'healthy',
  restarts_in_window: 0,
  window_seconds: 900,
  lines: [],
  total_lines: 0,
}
let stuck: StuckReport | undefined
vi.mock('../../queries/investigate', () => ({
  useFailureContext: () => ({ data: failure }),
}))
vi.mock('../../queries/stuck', () => ({
  useStuck: () => ({ data: stuck, refetch: vi.fn(), isFetching: false }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['fixit'],
  defaultNS: 'fixit',
  resources: { en: { fixit: fixitEn } },
  interpolation: { escapeValue: false },
})

const view = () =>
  render(
    <I18nextProvider i18n={i18n}>
      <DiagnosisCard appName="web" />
    </I18nextProvider>,
  )

const report = (over: Partial<StuckReport>): StuckReport => ({
  kind: 'app',
  name: 'web',
  ready: false,
  stuck: true,
  not_ready_seconds: 600,
  threshold_seconds: 300,
  timeline: [],
  causes: [],
  ...over,
})

describe('DiagnosisCard', () => {
  it('renders nothing for a healthy app', () => {
    stuck = undefined
    failure = { ...failure, state: 'healthy' }
    const { container } = view()
    expect(container).toBeEmptyDOMElement()
  })

  it('leads with the top cause and folds the log lines away', () => {
    stuck = report({
      causes: [
        {
          code: 'a',
          title: 'Port mismatch',
          explanation: 'Nothing listens on 8080.',
          confidence: 'high',
          evidence: [],
          actions: [{ type: 'fix', label: 'Fix the port', fix_id: 'f1' }],
        },
        {
          code: 'b',
          title: 'Slow start',
          explanation: '',
          confidence: 'medium',
          evidence: [],
          actions: [],
        },
      ],
    })
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
    view()
    expect(screen.getAllByRole('heading')).toHaveLength(1)
    expect(screen.getByText('Port mismatch')).toBeInTheDocument()
    expect(screen.getByText('Fix the port')).toBeInTheDocument()
    expect(screen.getByText('Open logs')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Re-check' })).toBeInTheDocument()
    expect(screen.queryByLabelText(/Last log lines/)).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Failure log' }))
    const lines = screen.getByLabelText(
      'Last log lines of the failing container',
    )
    expect(within(lines).getByText('panic: boom')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Copy lines' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Slow start')).toBeNull()
    fireEvent.click(
      screen.getByRole('button', { name: /Other possible causes/ }),
    )
    expect(screen.getByText('Slow start')).toBeInTheDocument()
  })

  it('offers Retry deploy and the build log for a failed deploy without a cause', () => {
    stuck = undefined
    failure = {
      state: 'deploy_failed',
      restarts_in_window: 0,
      window_seconds: 900,
      lines: [],
      total_lines: 0,
      deploy: { id: 'dpl_1', image: 'web:v2', error: 'probe failed' },
    } as unknown as FailureContext
    view()
    expect(screen.getByText('The latest deploy failed')).toBeInTheDocument()
    expect(screen.getByText('Retry deploy')).toBeInTheDocument()
    expect(screen.getByText('View the build log')).toBeInTheDocument()
  })
})
