import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposeSchedulesCard } from './ComposeSchedulesCard'
import composeEn from '../locales/en/compose.json'
import jobsEn from '../locales/en/jobs.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['compose', 'jobs'],
  defaultNS: 'compose',
  resources: { en: { compose: composeEn, jobs: jobsEn } },
  interpolation: { escapeValue: false },
})

const base = '/api/v1/apps/stack/compose/services/web/schedules/sct_1'
const routes: Record<string, { status: number; body: unknown }> = {
  'GET /api/v1/apps/stack/compose/schedules': {
    status: 200,
    body: {
      schedules: [
        {
          id: 'sct_1',
          service: 'web',
          name: 'report',
          command: ['sh', '-c', 'echo hi'],
          schedule: '@hourly',
          enabled: true,
        },
      ],
    },
  },
  [`POST ${base}/run`]: { status: 202, body: { id: 'sct_1', run_id: 'run_9' } },
  [`GET ${base}/runs?limit=10`]: {
    status: 200,
    body: [
      {
        id: 'run_9',
        trigger: 'manual',
        status: 'failed',
        started_at: new Date().toISOString(),
        exit_code: 3,
        duration_ms: 12,
      },
    ],
  },
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ComposeSchedulesCard', () => {
  it('runs a schedule now and shows its exit code in history', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      const hit = routes[`${init?.method ?? 'GET'} ${url}`] ?? {
        status: 404,
        body: { error: 'not found' },
      }
      return Promise.resolve({
        ok: hit.status < 400,
        status: hit.status,
        json: () => Promise.resolve(hit.body),
        text: () => Promise.resolve(JSON.stringify(hit.body)),
        headers: new Headers({ 'Content-Type': 'application/json' }),
      } as unknown as Response)
    })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <ComposeSchedulesCard appId="stack" />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    const run = await screen.findByRole('button', {
      name: 'Run report on web now',
    })
    await userEvent.click(run)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(`${base}/run`, { method: 'POST' })
    })
    expect(await screen.findByText('exit 3')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
  })
})
