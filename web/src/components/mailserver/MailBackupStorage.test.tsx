import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { MailBackupPanel } from './MailBackupPanel'
import { MailStoragePanel } from './MailStoragePanel'

vi.mock('../env/StepUpDialog', () => ({ StepUpDialog: () => null }))
vi.mock('../../hooks/useCanWriteSensitive', () => ({
  useCanWriteSensitive: () => true,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  interpolation: { escapeValue: false },
})

const list = {
  app: 'mail',
  encrypted: true,
  folder: '/data/mail-backups/mail',
  cap_bytes: 1000,
  used_bytes: 250,
  backups: [
    {
      id: 'b1',
      created_at: '2026-10-29T10:00:00Z',
      stored_bytes: 2048,
      data_bytes: 4096,
      files: 12,
      mox_version: 'v0.0.15',
      verified: true,
      where: 'local',
    },
  ],
}

const calls: { url: string; method: string; body: string }[] = []

function mockApi(storage: unknown = {}) {
  calls.length = 0
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({
        url,
        method,
        body: typeof init?.body === 'string' ? init.body : '',
      })
      const json = (b: unknown, status = 200) =>
        Promise.resolve(new Response(JSON.stringify(b), { status }))
      if (url.includes('/restore'))
        return json({
          app: 'mail',
          dry_run: true,
          backup_id: 'b1',
          mox_version: 'v0.0.15',
          files: 12,
          bytes: 4096,
        })
      if (url.includes('/mail/backups') && method === 'POST')
        return json(
          {
            id: 'j1',
            kind: 'backup',
            state: 'running',
            step: 'version',
            started_at: '',
          },
          202,
        )
      if (url.includes('/mail/backups')) return json(list)
      if (url.includes('/mail/storage')) return json(storage)
      return json({})
    }),
  )
}

function wrap(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('MailBackupPanel', () => {
  it('lists a verified backup and posts on back up now', async () => {
    mockApi()
    wrap(<MailBackupPanel app="mail" />)
    expect(await screen.findByText('b1')).toBeInTheDocument()
    expect(screen.getAllByText('Verified').length).toBeGreaterThan(0)
    await userEvent.click(screen.getByRole('button', { name: 'Back up now' }))
    await waitFor(() =>
      expect(
        calls.some(
          (c) => c.method === 'POST' && c.url === '/api/v1/mail/backups',
        ),
      ).toBe(true),
    )
  })

  it('dry-runs before restore and needs the typed id', async () => {
    mockApi()
    wrap(<MailBackupPanel app="mail" />)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Restore backup b1' }),
    )
    expect(await screen.findByText(/12 files/)).toBeInTheDocument()
    const dry = calls.find((c) => c.url.includes('/restore'))
    expect(dry?.body).not.toContain('confirm')
    const submit = screen.getByRole('button', { name: 'Restore now' })
    expect(submit).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/Type b1 to confirm/), 'b1')
    expect(submit).toBeEnabled()
  })
})

describe('MailStoragePanel', () => {
  it('shows per-step results and the local-only note', async () => {
    mockApi({
      app: 'mail',
      kind: 'local',
      folder: '/data/mail-backups/mail',
      cap_bytes: 1024,
      steps: [
        { name: 'put', ok: true },
        { name: 'get', ok: false, error: 'denied' },
      ],
      ok: false,
      saved: false,
      bucket_uploads: false,
    })
    wrap(<MailStoragePanel app="mail" />)
    expect(await screen.findByText(/Write passed/)).toBeInTheDocument()
    expect(screen.getByText(/Read failed/)).toBeInTheDocument()
    expect(screen.getByText('denied')).toBeInTheDocument()
    expect(
      screen.getByText(/still written to the local folder/),
    ).toBeInTheDocument()
  })
})
