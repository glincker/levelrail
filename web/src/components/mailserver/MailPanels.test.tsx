import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { MailPanels } from './MailPanels'

vi.mock('../env/StepUpDialog', () => ({ StepUpDialog: () => null }))
vi.mock('../../hooks/useCanWriteSensitive', () => ({
  useCanWriteSensitive: () => true,
}))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  interpolation: { escapeValue: false },
})

const box = (over: Record<string, unknown> = {}) => ({
  address: 'jo@example.com',
  domain: 'example.com',
  localpart: 'jo',
  team_id: 't1',
  suspended: false,
  suspend_pending: false,
  created_at: '2026-10-01T00:00:00Z',
  ...over,
})

function mockApi(opts: { boxes?: unknown[]; queueStatus?: number } = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const json = (b: unknown, status = 200) =>
        Promise.resolve(new Response(JSON.stringify(b), { status }))
      if (url.includes('/mail/status'))
        return json({
          installed: true,
          container_state: 'running',
          admin_reachable: true,
          tls: '',
          next_step: '',
        })
      if (url.includes('/mail/domains'))
        return json([
          {
            domain: 'example.com',
            service: 'mail',
            team_id: 't1',
            created_at: '',
          },
        ])
      if (url.includes('/mail/aliases')) return json([])
      if (url.includes('/mail/queue'))
        return opts.queueStatus && opts.queueStatus !== 200
          ? json({ error: 'nope' }, opts.queueStatus)
          : json({ app: 'mail', queued: 0, entries: [] })
      if (url.includes('/api/v1/teams'))
        return json([{ id: 't1', name: 'Core', default: true }])
      if (url.includes('/api/v1/users')) return json([])
      if (url.endsWith('/mail/mailboxes') && init?.method === 'POST')
        return json({ mailbox: box(), password: 'S3cretPw', notice: '' }, 201)
      if (url.endsWith('/mail/mailboxes')) return json(opts.boxes ?? [])
      return json({})
    }),
  )
}

function renderPanels() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MailPanels />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('MailPanels', () => {
  it('shows a generated password once and drops it on dismiss', async () => {
    mockApi()
    renderPanels()
    await userEvent.type(await screen.findByLabelText('Name'), 'jo')
    await waitFor(() =>
      expect(
        screen
          .getByRole('button', { name: 'Add mailbox' })
          .hasAttribute('disabled'),
      ).toBe(false),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Add mailbox' }))
    expect(await screen.findByText('S3cretPw')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'I saved it' }))
    expect(screen.queryByText('S3cretPw')).toBeNull()
  })

  it('warns plainly about a pending suspend', async () => {
    mockApi({ boxes: [box({ suspend_pending: true })] })
    renderPanels()
    expect(await screen.findByText('Suspend pending')).toBeTruthy()
    expect(screen.getByText(/not applied yet/)).toBeTruthy()
  })

  it('hides the queue for callers who are not instance members', async () => {
    mockApi({ queueStatus: 403 })
    renderPanels()
    await screen.findByRole('region', { name: 'Mailboxes' })
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some((c) => (c[0] as string).includes('/mail/queue')),
      ).toBe(true),
    )
    expect(screen.queryByRole('region', { name: 'Queue' })).toBeNull()
  })

  it('shows the queue for instance members', async () => {
    mockApi()
    renderPanels()
    expect(await screen.findByRole('region', { name: 'Queue' })).toBeTruthy()
  })

  it('requires the address typed before delete is enabled', async () => {
    mockApi({ boxes: [box()] })
    renderPanels()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Delete jo@example.com' }),
    )
    const confirm = await screen.findByRole('button', { name: 'Delete' })
    expect(confirm.hasAttribute('disabled')).toBe(true)
    await userEvent.type(
      screen.getByRole('textbox', { name: /jo@example.com|type/i }),
      'jo@example.com',
    )
    expect(confirm.hasAttribute('disabled')).toBe(false)
  })
})
