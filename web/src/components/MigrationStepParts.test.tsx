import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppImportRun } from './AppImportRun'
import { MigrationDomainProbes } from './MigrationDomainProbes'
import { MigrationLiveProgress } from './MigrationLiveProgress'
import { MigrationSourceTest } from './MigrationSourceTest'
import migrationEn from '../locales/en/migration.json'
import type {
  AppImportItem,
  AppImportSession,
  AppImportSource,
} from '../queries/appImport'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))

function bodyOf(init: RequestInit | undefined): unknown {
  return typeof init?.body === 'string' ? JSON.parse(init.body) : null
}

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

function item(
  name: string,
  state: AppImportItem['state'],
  reason = '',
): AppImportItem {
  return {
    source_id: `id-${name}`,
    name,
    target: name,
    state,
    selected: true,
    reason,
    entry: {
      name,
      kind: 'app',
      verdict: 'ready',
      source: 'image',
      env: { plain: 0, secret: 0, empty: 0 },
    },
  } as unknown as AppImportItem
}

function session(
  items: AppImportItem[],
  over: Partial<AppImportSession> = {},
): AppImportSession {
  return {
    id: 'appimp-1',
    platform: 'dokploy',
    connected: true,
    running: false,
    items,
    states: {},
    ...over,
  } as unknown as AppImportSession
}

function stubFetch(body: unknown, status = 200) {
  const fn = vi.fn<typeof fetch>(() =>
    Promise.resolve({
      ok: status < 400,
      status,
      json: () => Promise.resolve(body),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

function wrap(node: ReactNode) {
  const queryClient = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

const source: AppImportSource = {
  platform: 'dokploy',
  url: 'https://d.example.com',
  token: 'fake-token',
}

describe('MigrationSourceTest', () => {
  it('summarises what a reachable source holds', async () => {
    const user = userEvent.setup()
    const fn = stubFetch({
      ok: true,
      platform: 'dokploy',
      apps: 3,
      databases: 2,
      unsupported: 1,
      servers: ['main'],
      volumes: 4,
      domains: 5,
      crons: 6,
      warnings: [],
    })
    wrap(<MigrationSourceTest name="Dokploy" build={() => source} />)
    await user.click(screen.getByRole('button', { name: 'Test connection' }))
    expect(
      await screen.findByText(
        /Found 3 app\(s\), 2 database\(s\), 4 volume\(s\), 5 domain\(s\) and 6 cron job\(s\)/,
      ),
    ).toBeTruthy()
    expect(fn.mock.calls[0]?.[0]).toBe('/api/v1/migration/apps/test')
  })

  it('explains a rejected credential in plain words', async () => {
    const user = userEvent.setup()
    stubFetch({
      ok: false,
      platform: 'dokploy',
      failure: 'credentials',
      message: 'source returned HTTP 401',
      apps: 0,
      databases: 0,
      unsupported: 0,
      servers: [],
      volumes: 0,
      domains: 0,
      crons: 0,
      warnings: [],
    })
    wrap(<MigrationSourceTest name="Dokploy" build={() => source} />)
    await user.click(screen.getByRole('button', { name: 'Test connection' }))
    expect(await screen.findByText(/rejected the credential/)).toBeTruthy()
  })
})

describe('MigrationDomainProbes', () => {
  it('checks DNS and shows the result', async () => {
    const user = userEvent.setup()
    const fn = stubFetch({
      domain: 'app.example.com',
      app: 'web',
      targets: ['198.51.100.4'],
      ok: false,
      dns: {
        id: 'resolves',
        status: 'fail',
        detail: 'app.example.com resolves to 203.0.113.9, not this node',
        fix: 'update the record',
      },
    })
    wrap(
      <MigrationDomainProbes
        sessionId="appimp-1"
        domain="app.example.com"
        targetIp="198.51.100.4"
      />,
    )
    await user.click(screen.getByRole('button', { name: 'Check DNS now' }))
    expect(await screen.findByText(/resolves to 203.0.113.9/)).toBeTruthy()
    const [url, init] = fn.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe(
      '/api/v1/migration/apps/sessions/appimp-1/probe?target_ip=198.51.100.4',
    )
    expect(bodyOf(init)).toEqual({
      domain: 'app.example.com',
      kind: 'dns',
    })
  })

  it('checks that the app answers from this server', async () => {
    const user = userEvent.setup()
    const fn = stubFetch({
      domain: 'a.example.com',
      app: 'web',
      targets: [],
      ok: true,
      http: [
        { id: 'certificate', status: 'pass', detail: 'trusted certificate' },
        { id: 'health', status: 'pass', detail: 'GET / answered 200' },
      ],
    })
    wrap(<MigrationDomainProbes sessionId="appimp-1" domain="a.example.com" />)
    await user.click(
      screen.getByRole('button', { name: 'Check the app responds here' }),
    )
    expect(await screen.findByText('GET / answered 200')).toBeTruthy()
    const init = fn.mock.calls[0]?.[1] as unknown as RequestInit
    expect(bodyOf(init)).toMatchObject({ kind: 'http' })
  })
})

describe('MigrationLiveProgress', () => {
  it('reports counts in a polite live region', () => {
    wrap(
      <MigrationLiveProgress
        verifyView={false}
        active={false}
        session={session([
          item('web', 'staged'),
          item('api', 'stage-failed'),
          item('db', 'planned'),
        ])}
      />,
    )
    const status = screen.getByRole('status')
    expect(status.textContent).toBe('1 of 3 done, 1 failed')
    expect(status.getAttribute('aria-live')).toBe('polite')
    expect(screen.getByRole('progressbar')).toBeTruthy()
  })
})

describe('AppImportRun retry', () => {
  it('retries only the failed item', async () => {
    const user = userEvent.setup()
    const fn = stubFetch({ id: 'appimp-1', items: [], states: {} })
    wrap(
      <AppImportRun
        verifyView={false}
        onBack={vi.fn()}
        onNext={vi.fn()}
        session={session([
          item('web', 'staged'),
          item('api', 'stage-failed', 'image pull denied'),
        ])}
      />,
    )
    expect(screen.getByText('image pull denied')).toBeTruthy()
    const retry = screen.getAllByRole('button', { name: 'Retry' })
    expect(retry).toHaveLength(1)
    await user.click(retry[0] as HTMLElement)
    await waitFor(() => expect(fn).toHaveBeenCalled())
    const [url, init] = fn.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/migration/apps/sessions/appimp-1/stage')
    expect(bodyOf(init)).toEqual({ items: ['id-api'] })
  })

  it('lets a build orphaned by a restart be verified again', async () => {
    const user = userEvent.setup()
    const fn = stubFetch({ id: 'appimp-1', items: [], states: {} })
    wrap(
      <AppImportRun
        verifyView
        onBack={vi.fn()}
        onNext={vi.fn()}
        session={session([item('web', 'building')], {
          states: { building: 1 },
        })}
      />,
    )
    expect(
      screen.getByText('Interrupted by a restart. Verify it again.'),
    ).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Verify again' }))
    await waitFor(() => expect(fn).toHaveBeenCalled())
    const [url] = fn.mock.calls[0] as unknown as [string]
    expect(url).toBe('/api/v1/migration/apps/sessions/appimp-1/verify')
  })
})
