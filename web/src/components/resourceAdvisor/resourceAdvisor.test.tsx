import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/resourceAdvisor.json'
import { FindingCard } from './FindingCard'
import { OptimisePanel } from './OptimisePanel'
import { ResourceAdvisorTile } from './ResourceAdvisorTile'
import type { AdvisorFinding } from '../../types/resourceAdvisor'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

let experimental: string[] = []
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => experimental,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['resourceAdvisor'],
  defaultNS: 'resourceAdvisor',
  resources: { en: { resourceAdvisor: en } },
  interpolation: { escapeValue: false },
})

const MiB = 1024 * 1024

function finding(over: Partial<AdvisorFinding> = {}): AdvisorFinding {
  return {
    id: 'memory_limit_missing:database:askverdict',
    kind: 'memory_limit_missing',
    severity: 'suggest',
    resource: { kind: 'database', name: 'askverdict' },
    title: 'Set a memory limit on askverdict',
    why: 'askverdict has no memory limit.',
    evidence: {
      window: 'last 7 days',
      window_seconds: 604800,
      samples: 72,
      unit: 'bytes',
      p95: 220 * MiB,
      max: 230 * MiB,
    },
    impact: { risk: 'Without a limit the kernel picks the OOM victim.' },
    action: { type: 'set_memory_limit', value: 384 * MiB },
    params: { name: 'askverdict' },
    appliable: true,
    can_apply: true,
    ...over,
  }
}

type Routes = Record<string, unknown>

function stubFetch(routes: Routes) {
  const fn = vi.fn((input: string, init?: RequestInit) => {
    void init
    const url = input
    const key = Object.keys(routes).find((k) => url.startsWith(k))
    return Promise.resolve({
      ok: key !== undefined,
      status: key ? 200 : 404,
      json: () => Promise.resolve(key ? routes[key] : { error: 'nope' }),
    } as unknown as Response)
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderWith(ui: React.ReactNode) {
  const client = createTestQueryClient()
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>{ui}</QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  experimental = []
})

describe('FindingCard', () => {
  it('shows a translated title, why and the apply action', () => {
    renderWith(<FindingCard finding={finding()} />)
    expect(
      screen.getByText('Set a memory limit on askverdict'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('askverdict has no memory limit.'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Review and apply' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Suggested 384 MiB')).toBeInTheDocument()
  })

  it('hides apply when the caller cannot change the resource', () => {
    renderWith(<FindingCard finding={finding({ can_apply: false })} />)
    expect(
      screen.queryByRole('button', { name: 'Review and apply' }),
    ).toBeNull()
    expect(
      screen.getByText(/you can view this suggestion/i),
    ).toBeInTheDocument()
  })

  it('lists engine settings for a tuning finding without an apply button', () => {
    renderWith(
      <FindingCard
        finding={finding({
          id: 'engine_tuning:database:askverdict',
          kind: 'engine_tuning',
          title: 'Tune postgres settings for askverdict',
          params: { name: 'askverdict', engine: 'postgres' },
          appliable: false,
          can_apply: false,
          action: {
            type: 'manual',
            settings: { shared_buffers: '96MB' },
            instructions: 'Run ALTER SYSTEM',
          },
        })}
      />,
    )
    expect(screen.getByText('shared_buffers')).toBeInTheDocument()
    expect(screen.getByText('96MB')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Review and apply' }),
    ).toBeNull()
  })

  it('previews before applying and applies only on confirm', async () => {
    const fetchFn = stubFetch({
      '/api/v1/resource-advisor/findings/': {
        finding_id: 'x',
        resource: { kind: 'database', name: 'askverdict' },
        action: { type: 'set_memory_limit', value: 384 * MiB },
        before: { memory_bytes: 0, nano_cpus: 0 },
        after: { memory_bytes: 384 * MiB, nano_cpus: 0 },
        confirm_required: true,
        applied: false,
        applied_live: false,
      },
    })
    renderWith(<FindingCard finding={finding()} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'Review and apply' }),
    )
    expect(await screen.findByText('Memory limit')).toBeInTheDocument()
    const bodies = () => fetchFn.mock.calls.map((c) => c[1]?.body)
    expect(bodies()).toEqual([JSON.stringify({ confirm: false })])
    await userEvent.click(screen.getByRole('button', { name: 'Apply change' }))
    await waitFor(() =>
      expect(bodies()).toContain(JSON.stringify({ confirm: true })),
    )
  })
})

describe('OptimisePanel', () => {
  it('shows the plain limit suggestion and never calls the advisor when the beta is off', async () => {
    const fetchFn = stubFetch({
      '/api/v1/databases/main/resource-recommendation': {
        service_name: 'main',
        lookback_window: '168h0m0s',
        lookback_label: 'last 7 days',
        memory: {
          dimension: 'memory',
          sample_count: 0,
          data_sufficient: false,
          confidence: 'low',
          current_limit: 0,
          p95_usage: 0,
          p99_usage: 0,
          suggested_limit: 0,
          action: '',
          reason: 'No data yet.',
        },
        cpu: {
          dimension: 'cpu',
          sample_count: 0,
          data_sufficient: false,
          confidence: 'low',
          current_limit: 0,
          p95_usage: 0,
          p99_usage: 0,
          suggested_limit: 0,
          action: '',
          reason: 'No data yet.',
        },
      },
    })
    renderWith(<OptimisePanel databaseName="main" />)
    expect(
      await screen.findByText(/Based on the last 7 days/),
    ).toBeInTheDocument()
    expect(screen.queryByText('Beta')).toBeNull()
    expect(
      fetchFn.mock.calls.some((c) => c[0].includes('resource-advisor')),
    ).toBe(false)
  })

  it('shows advisor findings with a Beta badge when the beta is on', async () => {
    experimental = ['resource-advisor']
    stubFetch({
      '/api/v1/resource-advisor/findings': {
        generated_at: '',
        findings: [finding()],
      },
    })
    renderWith(<OptimisePanel databaseName="askverdict" />)
    expect(
      await screen.findByText('Set a memory limit on askverdict'),
    ).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
  })
})

describe('ResourceAdvisorTile', () => {
  it('renders nothing when the beta is off', () => {
    stubFetch({})
    const { container } = renderWith(<ResourceAdvisorTile />)
    expect(container).toBeEmptyDOMElement()
  })

  it('summarises findings when on', async () => {
    experimental = ['resource-advisor']
    stubFetch({
      '/api/v1/resource-advisor/summary': {
        generated_at: '',
        total: 9,
        by_severity: { suggest: 7 },
        by_kind: {},
        actionable: 7,
        pending_data: 0,
        saving_memory_bytes: 0,
        saving_disk_bytes: 0,
        top_savings: [],
      },
    })
    renderWith(<ResourceAdvisorTile />)
    expect(await screen.findByText(/9 suggestions/)).toBeInTheDocument()
  })
})
