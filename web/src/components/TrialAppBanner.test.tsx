import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../locales/en/common.json'
import trialAppEn from '../locales/en/trialApp.json'
import { TrialAppBanner } from './TrialAppBanner'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('./DeleteAppDialog', () => ({ DeleteAppDialog: () => null }))
const toastAdd = vi.fn()
vi.mock('@/components/ui/toast', () => ({
  toast: { add: (t: unknown) => toastAdd(t) as unknown },
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['trialApp', 'common'],
  defaultNS: 'trialApp',
  resources: { en: { trialApp: trialAppEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const app = {
  name: 'ntfy-a',
  domains: [] as string[],
  volumes: [
    { name: 'cache', container_path: '/var/cache/ntfy' },
    { name: 'data', container_path: '/etc/ntfy' },
  ],
  project_id: 'prj_1',
  environment_id: 'env_1',
}

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function stubFetch(keepStatus = 200) {
  const calls: string[] = []
  vi.stubGlobal('fetch', (input: string, init?: RequestInit) => {
    calls.push(`${init?.method ?? 'GET'} ${input}`)
    if (input.endsWith('/keep')) {
      return keepStatus === 200
        ? json({
            name: 'ntfy-a',
            kept: ['ntfy-a'],
            kept_at: '2026-10-14T12:00:00Z',
            already_kept: false,
          })
        : json({ error: 'app is not a trial deployment' }, keepStatus)
    }
    if (input.includes('/volumes/cache/backup-schedule')) {
      return json({
        service_name: 'ntfy-a',
        volume_name: 'cache',
        schedule: '0 3 * * *',
      })
    }
    if (input.includes('/backup-schedule')) {
      return json({ service_name: 'ntfy-a', volume_name: 'data' })
    }
    if (input.endsWith('/alerts')) return json([])
    if (input === '/api/v1/projects')
      return json([{ id: 'prj_1', name: 'Ops', created_at: '' }])
    if (input.includes('/environments')) {
      return json([{ id: 'env_1', project_id: 'prj_1', name: 'production' }])
    }
    return json({})
  })
  return calls
}

function renderBanner() {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <TrialAppBanner app={app} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('TrialAppBanner', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    toastAdd.mockReset()
  })

  it('offers keep alongside delete', () => {
    stubFetch()
    renderBanner()
    expect(
      screen.getByRole('button', { name: /Keep this app/ }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Stop & delete/ }),
    ).toBeInTheDocument()
  })

  it('explains what changes and shows the live checklist', async () => {
    stubFetch()
    renderBanner()
    await userEvent.click(screen.getByRole('button', { name: /Keep this app/ }))
    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByText(/stops being flagged as a throwaway trial/),
    ).toBeInTheDocument()
    expect(
      await within(dialog).findByText('1 of 2 volumes scheduled'),
    ).toBeInTheDocument()
    expect(
      await within(dialog).findByText('Ops / production'),
    ).toBeInTheDocument()
    expect(
      await within(dialog).findByText('Nothing notifies you when it breaks'),
    ).toBeInTheDocument()
    expect(
      within(dialog).getByText('Add a domain').closest('a'),
    ).toHaveAttribute('href', '/apps/$name/domains')
    expect(
      within(dialog).getByText('Add an alert rule').closest('a'),
    ).toHaveAttribute('href', '/apps/$name/alerts')
  })

  it('keeps the app and confirms with a toast', async () => {
    const calls = stubFetch()
    renderBanner()
    await userEvent.click(screen.getByRole('button', { name: /Keep this app/ }))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Keep app' }),
    )
    await waitFor(() =>
      expect(calls).toContain('POST /api/v1/apps/ntfy-a/keep'),
    )
    await waitFor(() =>
      expect(toastAdd).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'ntfy-a is now a permanent app.',
          type: 'success',
        }),
      ),
    )
  })

  it('shows the server error when keep is refused', async () => {
    stubFetch(409)
    renderBanner()
    await userEvent.click(screen.getByRole('button', { name: /Keep this app/ }))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Keep app' }),
    )
    expect(
      await screen.findByText(/not a trial deployment/),
    ).toBeInTheDocument()
    expect(toastAdd).not.toHaveBeenCalled()
  })
})
