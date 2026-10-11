import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppImportCutoverGuide } from './AppImportCutoverGuide'
import migrationEn from '../locales/en/migration.json'
import type { CutoverPlan, CutoverRun } from '../queries/appImportCutover'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

const plan: CutoverPlan = {
  app: 'web',
  verdict: 'warnings',
  health_path: '/healthz',
  checks: [
    {
      id: 'image',
      title: 'Image present and content verified',
      status: 'pass',
      detail: 'pulled from its registry',
    },
    {
      id: 'volumes',
      title: 'Volumes copied',
      status: 'warn',
      detail: '1 of 1 volume(s) not confirmed as copied: data',
      fix: { summary: 'Copy the volumes first.' },
    },
  ],
  domains: [
    { domain: 'web.example.com', method: 'dns', current: ['203.0.113.9'] },
  ],
}

const liveRun: CutoverRun = {
  id: 'cut_1',
  app: 'web',
  mode: 'switch',
  state: 'live',
  domains: [],
  steps: [
    {
      name: 'switch',
      state: 'done',
      detail: 'record changed',
      duration_ms: 40,
    },
  ],
  created_at: '2026-10-10T10:00:00Z',
  updated_at: '2026-10-10T10:00:05Z',
  rollbackable: true,
  in_flight: false,
}

function mockFetch(runs: CutoverRun[]) {
  const calls: { url: string; init?: RequestInit }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init })
      const body = url.endsWith('/plan')
        ? plan
        : init?.method === 'POST'
          ? liveRun
          : { runs }
      return Promise.resolve(
        new Response(JSON.stringify(body), { status: 200 }),
      )
    }),
  )
  return calls
}

function renderGuide() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <AppImportCutoverGuide sessionId="appimp-1" item="a1" app="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AppImportCutoverGuide', () => {
  it('shows the checklist and gates the switch on the typed app name', async () => {
    const calls = mockFetch([])
    renderGuide()
    await userEvent.click(
      screen.getByRole('button', { name: 'Guided cutover' }),
    )
    expect(await screen.findByText('Volumes copied')).toBeInTheDocument()
    expect(screen.getByText('Copy the volumes first.')).toBeInTheDocument()

    const sw = screen.getByRole('button', { name: 'Switch traffic' })
    expect(sw).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Type web to switch'), 'web')
    expect(sw).toBeDisabled()
    await userEvent.click(
      screen.getByRole('checkbox', { name: 'Go ahead despite the warnings' }),
    )
    await waitFor(() => expect(sw).toBeEnabled())
    await userEvent.click(sw)
    await waitFor(() =>
      expect(
        calls.some(
          (c) =>
            c.init?.method === 'POST' &&
            typeof c.init.body === 'string' &&
            c.init.body.includes('"mode":"switch"') &&
            c.init.body.includes('"confirm":"web"'),
        ),
      ).toBe(true),
    )
  })

  it('offers a one click rollback for a live run', async () => {
    const calls = mockFetch([liveRun])
    renderGuide()
    await userEvent.click(
      screen.getByRole('button', { name: 'Guided cutover' }),
    )
    const rb = await screen.findByRole('button', { name: 'Roll back' })
    await userEvent.click(rb)
    await waitFor(() =>
      expect(calls.some((c) => c.url.endsWith('/cut_1/rollback'))).toBe(true),
    )
  })
})
