import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import resilienceEn from '../../locales/en/resilience.json'
import type {
  BenchmarkComparison,
  BenchmarkList,
  BenchmarkRun,
} from '../../queries/benchmarks'
import { BenchmarksPage } from './BenchmarksPage'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'resilience'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, resilience: resilienceEn } },
  interpolation: { escapeValue: false },
})

const state = vi.hoisted(() => ({
  list: null as BenchmarkList | null,
  run: null as BenchmarkRun | null,
  compare: null as BenchmarkComparison | null,
  compareArgs: [] as (string | null)[],
}))

vi.mock('../../hooks/useBrand', () => ({
  useBrand: () => ({ BinaryName: 'tool' }),
}))
vi.mock('@/components/PageBanner', () => ({
  PageBanner: ({ action }: { action: React.ReactNode }) => <div>{action}</div>,
}))
vi.mock('../../queries/benchmarks', () => ({
  useBenchmarkRuns: () => ({
    data: state.list,
    isPending: false,
    error: null,
    refetch: vi.fn(),
    isFetching: false,
  }),
  useBenchmarkRun: () => ({ data: state.run, isPending: false, error: null }),
  useBenchmarkComparison: (base: string | null, head: string | null) => {
    state.compareArgs = [base, head]
    return { data: state.compare, isPending: false, error: null }
  },
  useDeleteBenchmark: () => ({ mutate: vi.fn(), isPending: false }),
}))

function summary(id: string, at: string, os: string) {
  return {
    id,
    generated_at: at,
    version: 'v1',
    commit: 'abcdef0123456789',
    dirty: false,
    host_summary: `${os}/arm64, 8 CPUs`,
    host_os: os,
    repeat: 1,
    passed: 3,
    failed: 0,
    skipped: 0,
    created_at: at,
  }
}

const report: BenchmarkRun['report'] = {
  schema_version: 1,
  generated_at: '2026-10-20T10:00:00Z',
  build: { version: 'v1', commit: 'abcdef0123456789', dirty: false },
  host: {
    os: 'darwin',
    arch: 'arm64',
    cpus: 8,
    docker: {},
    load1_start: 1,
    load1_end: 2,
  },
  repeat: 1,
  methodology: ['Open loop load.'],
  caveats: ['NOT Linux production numbers: the host is darwin.'],
  results: [
    {
      scenario: 'deploy-latency',
      status: 'passed',
      summary: 'cold 4900 ms',
      recovery_ms: 0,
      error_window_ms: 0,
      requests: 0,
      request_errors: 0,
      ran_at: '2026-10-20T10:00:00Z',
      metrics: { deploy_cold_ms: 4900 },
    },
  ],
  limits: [{ question: 'Anything on a Linux server', status: 'not measured' }],
}

describe('BenchmarksPage', () => {
  beforeEach(() => {
    state.list = {
      keep: 50,
      runs: [
        summary('bench_new', '2026-10-21T10:00:00Z', 'darwin'),
        summary('bench_old', '2026-10-20T10:00:00Z', 'darwin'),
      ],
    }
    state.run = {
      run: summary('bench_new', '2026-10-21T10:00:00Z', 'darwin'),
      report,
      groups: { 'deploy-latency': 'deploy' },
    }
    state.compare = {
      base: {
        id: 'bench_old',
        generated_at: '',
        version: 'v1',
        commit: 'a',
        dirty: false,
        host: 'h',
        runs: 1,
      },
      head: {
        id: 'bench_new',
        generated_at: '',
        version: 'v1',
        commit: 'b',
        dirty: false,
        host: 'h',
        runs: 1,
      },
      deltas: [
        {
          scenario: 'deploy-latency',
          group: 'deploy',
          metric: 'deploy_cold_ms',
          base: 5000,
          head: 4000,
          delta: -1000,
          delta_pct: -20,
          direction: 'better',
        },
      ],
      status_changes: [],
      warnings: [
        'At least one run was built from a tree with uncommitted changes.',
      ],
      noise_pct: 5,
      statistic: 'median over runs',
    }
  })

  it('always shows the standing caveat and lists runs', () => {
    render(<BenchmarksPage />)
    expect(screen.getByText('Not production capacity numbers')).toBeTruthy()
    expect(
      screen.getByText(/NOT Linux production numbers\. Read the caveats/),
    ).toBeTruthy()
    expect(screen.getAllByRole('row').length).toBe(3)
  })

  it('shows an empty state with the upload command', () => {
    state.list = { keep: 50, runs: [] }
    render(<BenchmarksPage />)
    expect(screen.getByText('No benchmark runs stored yet')).toBeTruthy()
    expect(
      screen.getByText(
        /bench chaos run all --repeat 5 --out results.json --upload/,
      ),
    ).toBeTruthy()
  })

  it('opens a run with its caveats, host and limits', async () => {
    render(<BenchmarksPage />)
    fireEvent.click(
      screen.getAllByRole('button', { name: 'View' })[0] as HTMLElement,
    )
    expect(
      await screen.findByText(
        'NOT Linux production numbers: the host is darwin.',
      ),
    ).toBeTruthy()
    expect(screen.getByText('Limits table')).toBeTruthy()
    expect(screen.getByText('Anything on a Linux server')).toBeTruthy()
    expect(screen.getByText('4900 ms')).toBeTruthy()
  })

  it('compares two selected runs older to newer', async () => {
    render(<BenchmarksPage />)
    const compare = screen.getByRole<HTMLButtonElement>('button', {
      name: 'Compare selected',
    })
    expect(compare.disabled).toBe(true)
    fireEvent.click(
      screen.getByRole('checkbox', {
        name: 'Select run bench_new for compare',
      }),
    )
    fireEvent.click(
      screen.getByRole('checkbox', {
        name: 'Select run bench_old for compare',
      }),
    )
    fireEvent.click(compare)
    expect(await screen.findByText('Better')).toBeTruthy()
    expect(screen.getByText('-20.0%')).toBeTruthy()
    expect(state.compareArgs).toEqual(['bench_old', 'bench_new'])
  })
})
