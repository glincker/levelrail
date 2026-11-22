import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import buildLogsEn from '../locales/en/buildLogs.json'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogSearchPanel } from './LogSearchPanel'
import { LogLevelGutter } from './LogLevelChips'
import {
  bucketOf,
  countLevels,
  filterByLevels,
  summarizeShown,
  toggleLevel,
  type LevelBucket,
} from '../lib/logLevels'
import type { LogEntry } from '../types/logs'
import { createTestQueryClient } from '@/test/testQueryClient'

function logEntry(message: string, level?: string): LogEntry {
  return {
    timestamp: '2026-09-26T10:00:00Z',
    stream: 'stdout',
    message,
    structured: false,
    level,
  }
}

const ENTRIES: LogEntry[] = [
  logEntry('boom', 'error'),
  logEntry('crash', 'fatal'),
  logEntry('careful', 'warn'),
  logEntry('hello', 'info'),
  logEntry('plain'),
]

const buildLogsI18n = i18next.createInstance()
void buildLogsI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['buildLogs'],
  defaultNS: 'buildLogs',
  resources: { en: { buildLogs: buildLogsEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('logLevels', () => {
  it.each([
    ['fatal', 'error'],
    ['error', 'error'],
    ['warn', 'warn'],
    ['info', 'info'],
    ['trace', 'debug'],
    ['debug', 'debug'],
    [undefined, 'none'],
    ['weird', 'none'],
  ] as const)('buckets %s as %s', (level, want) => {
    expect(bucketOf(level)).toBe(want)
  })

  it('counts and filters by bucket', () => {
    expect(countLevels(ENTRIES)).toEqual({
      error: 2,
      warn: 1,
      info: 1,
      debug: 0,
      none: 1,
    })
    const only = new Set<LevelBucket>(['warn', 'info'])
    expect(filterByLevels(ENTRIES, only).map((e) => e.message)).toEqual([
      'careful',
      'hello',
    ])
    expect(filterByLevels(ENTRIES, new Set())).toHaveLength(5)
  })

  it('toggles without mutating', () => {
    const base = new Set<LevelBucket>(['error'])
    const next = toggleLevel(base, 'warn')
    expect([...base]).toEqual(['error'])
    expect([...next].sort()).toEqual(['error', 'warn'])
    expect(toggleLevel(next, 'error').has('error')).toBe(false)
  })

  it.each([
    [5, 5, 5, false, 'Showing 5 of 5'],
    [1000, 1000, 4200, false, 'Showing 1,000 of 4,200'],
    [2, 5, 5, true, 'Showing 2 of 5 loaded'],
    [2, 1000, 4200, true, 'Showing 2 of 1,000 loaded (4,200 in range)'],
  ])(
    'summarizes %d/%d/%d filter=%s',
    (shown, loaded, total, filtered, want) => {
      expect(summarizeShown(shown, loaded, total, filtered)).toBe(want)
    },
  )
})

describe('LogLevelGutter', () => {
  it('shows the level word, not only a color', () => {
    render(<LogLevelGutter level="fatal" bucket="error" />)
    expect(screen.getByText('fatal')).toBeInTheDocument()
  })
})

describe('LogSearchPanel level filter', () => {
  it('shows counts, the total line and filters by level chip', async () => {
    const fetchMock = vi.fn<
      (input: string, init?: RequestInit) => Promise<Response>
    >(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ entries: ENTRIES, total: 9 }),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <LogSearchPanel appName="web" />
      </QueryClientProvider>,
    )

    expect(await screen.findByText('Showing 5 of 9')).toBeInTheDocument()
    const searchCall = fetchMock.mock.calls.find((c) =>
      c[0].includes('/api/v1/apps/web/logs?'),
    )
    expect(searchCall?.[0]).toContain('limit=1000')

    const errorChip = screen.getByRole('button', { name: /^error/ })
    expect(errorChip).toHaveTextContent('2')
    expect(screen.getByRole('button', { name: /^debug/ })).toBeDisabled()

    await userEvent.click(errorChip)
    expect(errorChip).toHaveAttribute('aria-pressed', 'true')
    expect(
      screen.getByText('Showing 2 of 5 loaded (9 in range)'),
    ).toBeInTheDocument()
  })

  it('searches the last hour first and all time on request', async () => {
    const fetchMock = vi.fn<
      (input: string, init?: RequestInit) => Promise<Response>
    >(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ entries: ENTRIES, total: 5 }),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={buildLogsI18n}>
          <LogSearchPanel appName="web" />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    await screen.findByText('Showing 5 of 5')
    const fromOf = (url: string) =>
      Date.parse(new URL(url, 'http://x').searchParams.get('from') ?? '')
    const searches = () =>
      fetchMock.mock.calls
        .map((c) => c[0])
        .filter((u) => u.includes('/api/v1/apps/web/logs?'))
    const first = searches()[0] ?? ''
    expect(Date.now() - fromOf(first)).toBeLessThan(61 * 60 * 1000)

    await userEvent.click(
      await screen.findByRole('button', { name: 'Search all time' }),
    )
    await screen.findByRole('button', { name: 'Back to last hour' })
    const last = searches().at(-1) ?? ''
    expect(fromOf(last)).toBe(0)
  })
})
