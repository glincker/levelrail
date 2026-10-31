import type { ReactNode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import backupsEn from '../../locales/en/backups.json'
import commonEn from '../../locales/en/common.json'
import { shouldPromptForBackupTarget, Route } from './index'
import type { Brand } from '../../types/brand'
import type { BackupCoverage, StorageOrphans } from '../../types/backupCoverage'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: '',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: '',
    DiscussionsURL: '',
    RepoURL: '',
  }),
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ to, children }: { to?: string; children?: ReactNode }) => (
      <a href={to}>{children}</a>
    ),
  }
})

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['backups', 'common'],
  defaultNS: 'backups',
  resources: { en: { backups: backupsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const PUBLIC_KEY = 'age1publickeyshouldneverprintinline'

function coverage(overrides: Partial<BackupCoverage> = {}): BackupCoverage {
  return {
    summary: {
      total: 2,
      protected: 1,
      verified: 1,
      unverified: 0,
      never_ran: 0,
      failing: 0,
      unprotected: 1,
      scratch: 0,
      storage_bytes: 2048,
      last_verified_restore: '2026-10-09T03:00:00Z',
      next_run: '2026-10-12T03:00:00Z',
    },
    resources: [
      {
        kind: 'database',
        name: 'orders',
        state: 'unprotected',
        reason: 'no backup schedule',
        primary_action: 'protect',
        total_bytes: 0,
        backup_count: 0,
        encrypted: false,
      },
      {
        kind: 'volume',
        app_name: 'web',
        name: 'uploads',
        state: 'verified',
        primary_action: 'restore',
        schedule: '0 3 * * *',
        target_id: 't1',
        last_backup_at: '2026-10-10T03:00:00Z',
        last_verified_at: '2026-10-09T03:00:00Z',
        total_bytes: 2048,
        backup_count: 3,
        encrypted: true,
      },
    ],
    targets: [{ id: 't1', name: 'offsite', provider: 'r2' }],
    defaults: { schedule: '0 3 * * *', retain: 7, retain_days: 30 },
    encryption: {
      enabled: true,
      recipient: PUBLIC_KEY,
      codec: 'zstd+age',
      key_stored_off_box: false,
    },
    ...overrides,
  }
}

const NO_ORPHANS: StorageOrphans = {
  orphans: [],
  total_bytes: 0,
  threshold_bytes: 1 << 30,
}

function stubFetch(cov: BackupCoverage, orphans: StorageOrphans = NO_ORPHANS) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      const body = url.includes('/backups/coverage')
        ? cov
        : url.includes('/storage/orphans')
          ? orphans
          : url.includes('/backups/health')
            ? { resources: [], targets: [], encryption: { enabled: false } }
            : url.includes('/backup-targets')
              ? []
              : []
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

function renderPage() {
  const queryClient = createTestQueryClient()
  const Page = Route.options.component!
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <Page />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('shouldPromptForBackupTarget', () => {
  it('is false while loading, true only with zero loaded targets', () => {
    expect(shouldPromptForBackupTarget(true, 0)).toBe(false)
    expect(shouldPromptForBackupTarget(false, 0)).toBe(true)
    expect(shouldPromptForBackupTarget(false, 1)).toBe(false)
  })
})

describe('backups page', () => {
  beforeEach(() => {
    vi.unstubAllGlobals()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('walks a first run: no destination shows the setup steps, not an empty table', async () => {
    stubFetch(coverage({ targets: [], resources: [] }))
    renderPage()

    expect(
      await screen.findByRole('heading', { name: 'Set up backups' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('radiogroup', { name: 'Provider' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Protect everything' }),
    ).not.toBeInTheDocument()
  })

  it('warns loudly when local disk is chosen as the destination', async () => {
    const user = userEvent.setup()
    stubFetch(coverage({ targets: [], resources: [] }))
    renderPage()

    await user.click(await screen.findByRole('radio', { name: /Local disk/ }))

    expect(screen.getByRole('alert')).toHaveTextContent(/lost with the machine/)
  })

  it('leads with coverage and one action, and lists every resource with its state', async () => {
    stubFetch(coverage())
    renderPage()

    expect(
      await screen.findByText('1 of 2 resources protected'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Protect everything' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Not protected')).toBeInTheDocument()
    expect(screen.getByText('Protected and verified')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Protect' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Restore' })).toBeInTheDocument()
  })

  it('stacks each resource as a card on narrow screens instead of a wide table', async () => {
    stubFetch(coverage())
    renderPage()
    await screen.findByText('1 of 2 resources protected')

    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    const rows = screen
      .getAllByRole('listitem')
      .filter((el) => el.className.includes('md:grid-cols-'))
    expect(rows).toHaveLength(2)
    for (const row of rows) {
      expect(row.className).not.toMatch(/\bmin-w-\[/)
    }
  })

  it('never prints the public key inline and offers a copy button in a disclosure', async () => {
    stubFetch(coverage())
    renderPage()
    await screen.findByText('1 of 2 resources protected')

    expect(document.body).not.toHaveTextContent(PUBLIC_KEY)
    expect(
      screen.getByText(/not confirmed that the private key is stored/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'I stored it off this server' }),
    ).toBeInTheDocument()
  })

  it('hides the key warning once the key is acknowledged', async () => {
    const cov = coverage()
    cov.encryption.key_stored_off_box = true
    stubFetch(cov)
    renderPage()
    await screen.findByText('1 of 2 resources protected')

    expect(
      screen.queryByText(/not confirmed that the private key is stored/),
    ).not.toBeInTheDocument()
  })

  it('keeps the attempts table collapsed until asked for', async () => {
    stubFetch(coverage())
    renderPage()
    await screen.findByText('1 of 2 resources protected')

    expect(screen.getByText('Recent activity')).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('requires the volume name typed before orphaned data is reclaimed', async () => {
    const user = userEvent.setup()
    stubFetch(coverage(), {
      orphans: [
        {
          id: 'db-gone-data',
          name: 'db-gone-data',
          size_bytes: 5 * 1024 * 1024,
          age_days: 12,
          belonged_to: { kind: 'database', name: 'gone', role: 'data' },
          backup_count: 0,
        },
      ],
      total_bytes: 5 * 1024 * 1024,
      threshold_bytes: 1 << 30,
    })
    renderPage()

    const panel = (
      await screen.findByRole('heading', { name: 'Orphaned data' })
    ).closest('section')!
    expect(await within(panel).findByText('No backup')).toBeInTheDocument()
    await user.click(within(panel).getByRole('button', { name: 'Reclaim' }))

    const confirm = await screen.findByRole('button', {
      name: 'Delete this data',
    })
    expect(confirm).toBeDisabled()
    await user.type(screen.getByRole('textbox'), 'db-gone-data')
    await waitFor(() => {
      expect(confirm).toBeEnabled()
    })
  })
})
