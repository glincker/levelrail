import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/mailServer.json'
import { avatarTone } from './mailAvatar'
import { MailPage } from './MailPage'
import { MailWebmailButton } from './MailWebmailButton'

vi.mock('../env/StepUpDialog', () => ({ StepUpDialog: () => null }))
vi.mock('../../hooks/useCanWriteSensitive', () => ({
  useCanWriteSensitive: () => true,
}))
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({
    actions,
    children,
  }: {
    actions: React.ReactNode
    children: React.ReactNode
  }) => (
    <div>
      {actions}
      {children}
    </div>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { mailServer: en } },
  defaultNS: 'mailServer',
  interpolation: { escapeValue: false },
})

const box = (localpart: string, over: Record<string, unknown> = {}) => ({
  address: `${localpart}@example.com`,
  domain: 'example.com',
  localpart,
  team_id: 't1',
  suspended: false,
  suspend_pending: false,
  created_at: '2026-10-01T00:00:00Z',
  ...over,
})

function mockApi(
  opts: { urls?: unknown; backups?: boolean; listStatus?: number } = {},
) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const json = (b: unknown, status = 200) =>
        Promise.resolve(new Response(JSON.stringify(b), { status }))
      if (url.includes('/mail/status'))
        return json({
          app: 'mail',
          installed: true,
          container_state: 'running',
          admin_reachable: true,
          tls: 'provided',
          next_step: '',
          domain: 'example.com',
        })
      if (url.includes('/apps/mail/urls'))
        return json(opts.urls ?? { addresses: [] })
      if (url.includes('/mail/domains'))
        return json([
          {
            domain: 'example.com',
            service: 'mail',
            team_id: 't1',
            created_at: '',
          },
        ])
      if (url.includes('/mail/dns/check')) return json({ items: [] })
      if (url.includes('/mail/backups'))
        return json({
          app: 'mail',
          encrypted: true,
          folder: '',
          cap_bytes: 1000,
          used_bytes: 0,
          backups: opts.backups
            ? [
                {
                  id: 'b1',
                  created_at: new Date().toISOString(),
                  stored_bytes: 2048,
                  data_bytes: 1,
                  files: 1,
                  mox_version: 'v1',
                  verified: true,
                  where: 'local',
                },
              ]
            : [],
        })
      if (url.includes('/mail/aliases')) return json([])
      if (url.includes('/mail/queue'))
        return json({ app: 'mail', queued: 0, entries: [] })
      if (url.includes('/api/v1/teams'))
        return json([{ id: 't1', name: 'Core', default: true }])
      if (url.includes('/api/v1/users')) return json([])
      if (url.includes('/password') && init?.method === 'POST')
        return json({ mailbox: box('jo'), password: 'NewPw123', notice: '' })
      if (url.endsWith('/mail/mailboxes') && opts.listStatus)
        return json(
          { error: 'your team role does not allow this' },
          opts.listStatus,
        )
      if (url.endsWith('/mail/mailboxes'))
        return json([box('jo'), box('ann', { suspended: true })])
      return json({})
    }),
  )
}

function renderPage(props: React.ComponentProps<typeof MailPage> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <MailPage {...props} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  window.localStorage.clear()
})

describe('Mail tabs', () => {
  it('opens the tab from the deep link and reports tab changes', async () => {
    mockApi()
    const onTab = vi.fn()
    renderPage({ tab: 'mailboxes', onTab })
    expect(await screen.findByLabelText('Search mailboxes')).toBeTruthy()
    await userEvent.click(screen.getByRole('tab', { name: 'Backups' }))
    expect(onTab).toHaveBeenCalledWith('backups')
    expect(window.localStorage.getItem('levelrail.mail.tab')).toBe('backups')
  })

  it('remembers the last tab when there is no deep link', async () => {
    window.localStorage.setItem('levelrail.mail.tab', 'mailboxes')
    mockApi()
    renderPage()
    expect(await screen.findByLabelText('Search mailboxes')).toBeTruthy()
  })
})

describe('Mailbox list', () => {
  it('searches and filters by status', async () => {
    mockApi()
    renderPage({ tab: 'mailboxes' })
    await screen.findByText('jo@example.com')
    await userEvent.type(screen.getByLabelText('Search mailboxes'), 'ann')
    expect(screen.queryByText('jo@example.com')).toBeNull()
    await userEvent.clear(screen.getByLabelText('Search mailboxes'))
    await userEvent.selectOptions(
      screen.getByLabelText('Filter by status'),
      'suspended',
    )
    expect(screen.queryByText('jo@example.com')).toBeNull()
    expect(screen.getByText('ann@example.com')).toBeTruthy()
  })

  it('opens the detail sheet, resets a password and asks to type the address to delete', async () => {
    mockApi()
    renderPage({ tab: 'mailboxes' })
    await userEvent.click(
      await screen.findByRole('button', { name: 'Details: jo@example.com' }),
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Reset password' }),
    )
    expect(await screen.findByText('NewPw123')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Delete jo@example.com' }),
    )
    const confirm = await screen.findByRole('button', { name: 'Delete' })
    expect(confirm.hasAttribute('disabled')).toBe(true)
  })

  it('adds a mailbox from the on-demand row with the install domain as a chip', async () => {
    mockApi()
    renderPage({ tab: 'mailboxes', add: true })
    expect(await screen.findByText('@example.com')).toBeTruthy()
    await userEvent.type(screen.getByLabelText('Name'), 'sam')
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Create' }).hasAttribute('disabled'),
      ).toBe(false),
    )
  })
})

describe('Open webmail', () => {
  it('links to the web listener in a new tab when a domain is attached', async () => {
    mockApi({
      urls: {
        addresses: [
          {
            kind: 'domain',
            url: 'https://mail.example.com',
            reachable_hint: 'reachable',
          },
        ],
      },
    })
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={qc}>
        <I18nextProvider i18n={i18n}>
          <MailWebmailButton app="mail" />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    const link = await screen.findByRole('link', { name: 'Open webmail' })
    expect(link.getAttribute('href')).toBe('https://mail.example.com/webmail/')
    expect(link.getAttribute('target')).toBe('_blank')
    expect(link.getAttribute('rel')).toContain('noopener')
  })

  it('is disabled with an explanation and a settings link without a URL', async () => {
    mockApi()
    renderPage()
    const btn = await screen.findByRole('button', { name: 'Open webmail' })
    expect(btn.hasAttribute('disabled')).toBe(true)
    expect(btn.closest('span')?.getAttribute('title')).toBe(
      'Attach a domain to the webmail listener to enable this',
    )
    expect(
      screen.getByRole('link', { name: 'App domain settings' }),
    ).toBeTruthy()
  })
})

describe('Overview all set', () => {
  it('settles without a DNS report once everything exists, with DNS as its own chip', async () => {
    mockApi({ backups: true })
    renderPage({ tab: 'overview' })
    expect(await screen.findByText('All set')).toBeTruthy()
    expect(screen.getByText('DNS: Not checked yet')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Check DNS' }))
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some((c) =>
            String(c[0] as string).startsWith(
              '/api/v1/mail/dns/check?domain=example.com',
            ),
          ),
      ).toBe(true),
    )
  })

  it('keeps asking for the next step while no backup exists', async () => {
    mockApi()
    renderPage({ tab: 'overview' })
    expect(await screen.findByText('DNS: Not checked yet')).toBeTruthy()
    expect(screen.queryByText('All set')).toBeNull()
  })
})

describe('Mailbox avatars and chips', () => {
  it('shows initials in a stable tone and a chip only for non-default states', async () => {
    mockApi()
    renderPage({ tab: 'mailboxes' })
    await screen.findByText('jo@example.com')
    const avatars = screen.getAllByTestId('mailbox-avatar')
    const first = avatars[0]
    expect(first?.textContent).toBe('JO')
    for (const cls of avatarTone('jo@example.com').split(' '))
      expect(first?.className).toContain(cls)
    const list = within(screen.getByRole('list', { name: en.mbx.list }))
    expect(list.getByText('Suspended')).toBeTruthy()
    expect(list.queryByText('Active')).toBeNull()
  })
})

describe('Mailbox add row and errors', () => {
  it('keeps the add row hidden until asked, then closes on Escape', async () => {
    mockApi()
    renderPage({ tab: 'mailboxes' })
    await screen.findByText('jo@example.com')
    expect(screen.queryByLabelText('Name')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Add mailbox' }))
    const name = await screen.findByLabelText('Name')
    expect(screen.queryByLabelText(en.mbx.add.user)).toBeNull()
    await userEvent.type(name, '{Escape}')
    await waitFor(() => expect(screen.queryByLabelText('Name')).toBeNull())
  })

  it('shows a calm view-only notice, not an error, when the list is forbidden', async () => {
    mockApi({ listStatus: 403 })
    renderPage({ tab: 'mailboxes' })
    expect(await screen.findByText(en.mbx.viewOnly)).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('lets the header actions wrap instead of clipping', async () => {
    mockApi()
    renderPage()
    const btn = await screen.findByRole('button', { name: 'Open webmail' })
    expect(btn.closest('div')?.className).toContain('flex-wrap')
  })
})
