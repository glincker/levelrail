import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { Suspense } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../../locales/en/common.json'
import gitConnectEn from '../../../locales/en/gitConnect.json'
import settingsEn from '../../../locales/en/settings.json'
import type { GitProviderConnection } from '../../../types/gitConnections'
import { createTestQueryClient } from '@/test/testQueryClient'
import { GitAccessPanel } from './GitAccessPanel'
import { GitCredentialStatus } from './GitCredentialStatus'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings', 'gitConnect'],
  defaultNS: 'common',
  resources: {
    en: { common: commonEn, settings: settingsEn, gitConnect: gitConnectEn },
  },
  interpolation: { escapeValue: false },
})

const NOW = Date.parse('2026-11-15T12:00:00Z')

function conn(extra: Partial<GitProviderConnection>): GitProviderConnection {
  return {
    id: 'c1',
    provider: 'gitlab',
    label: 'Main',
    host: 'gitlab.com',
    instance_url: 'https://gitlab.com',
    client_id: '',
    is_default: true,
    authorized: true,
    created_at: '2026-10-20T10:00:00Z',
    source_count: 0,
    connection: { status: 'ok' },
    authorize_path: '/api/v1/gitlab-app/connect?connection_id=c1',
    auth_method: 'token',
    credential_kind: 'static_token',
    can_rotate: true,
    ...extra,
  }
}

let rotateStatus = 200
let rotateBody: unknown
let posted: string | undefined

beforeEach(() => {
  rotateStatus = 200
  rotateBody = { rotated: true, expires_at: '2027-02-13', connection: conn({}) }
  posted = undefined
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      if (url.endsWith('/rotate')) {
        posted = init?.body as string | undefined
        return Promise.resolve(
          new Response(JSON.stringify(rotateBody), {
            status: rotateStatus,
            headers: { 'Content-Type': 'application/json' },
          }),
        )
      }
      return Promise.resolve(new Response('{}', { status: 404 }))
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderPanel(c: GitProviderConnection) {
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <Suspense fallback={null}>
          <GitAccessPanel
            provider="gitlab"
            connection={c}
            canSetDefault={false}
            onAddAnother={vi.fn()}
          />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

async function openDialog() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('button', { name: 'Rotate token' }))
  return user
}

describe('rotate flow', () => {
  it('confirms, then sends the prefilled expiry', async () => {
    renderPanel(conn({}))
    const user = await openDialog()
    expect(screen.getByText(/old one stops working at once/)).toBeTruthy()
    const input = screen.getByLabelText('New token expiry (YYYY-MM-DD)')
    expect((input as HTMLInputElement).value).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    await user.click(screen.getByRole('button', { name: 'Rotate token' }))
    await waitFor(() => expect(posted).toBeDefined())
    const body = JSON.parse(posted ?? '{}') as { expires_at?: string }
    expect(body.expires_at).toMatch(/^\d{4}-/)
  })

  it.each([
    [
      409,
      { error: 'self_rotate scope is missing' },
      'self_rotate scope is missing',
    ],
    [501, { error: 'x' }, /needs a master key/],
    [502, { error: 'x' }, /did not complete the rotation/],
  ])('shows a message for status %i', async (status, body, text) => {
    rotateStatus = status
    rotateBody = body
    renderPanel(conn({}))
    const user = await openDialog()
    await user.click(screen.getByRole('button', { name: 'Rotate token' }))
    expect(await screen.findByText(text)).toBeTruthy()
  })

  it('stays disabled with the reason for OAuth', () => {
    renderPanel(
      conn({
        can_rotate: false,
        credential_kind: 'oauth_auto_renew',
        auth_method: 'oauth',
      }),
    )
    const btn = screen.getByRole('button', { name: 'Rotate token' })
    expect(btn.hasAttribute('disabled')).toBe(true)
  })
})

describe('credential status', () => {
  it('shows auto-renew with last refresh and the failing line', async () => {
    const reauth = vi.fn()
    render(
      <I18nextProvider i18n={i18n}>
        <GitCredentialStatus
          kind="oauth_auto_renew"
          lastRefreshAt="2026-11-15T11:00:00Z"
          lastRefreshError="invalid_grant"
          now={NOW}
          onReauthorize={reauth}
        />
      </I18nextProvider>,
    )
    expect(screen.getByText('Auto-renews')).toBeTruthy()
    expect(screen.getByText('Refresh failing')).toBeTruthy()
    expect(screen.getByText('invalid_grant')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Re-authorize' }))
    expect(reauth).toHaveBeenCalled()
  })

  it('counts days, then hours under 48h, never invents an expiry', () => {
    const at = (ms: number) => new Date(NOW + ms).toISOString()
    const { rerender } = render(
      <I18nextProvider i18n={i18n}>
        <GitCredentialStatus
          kind="static_token"
          expiresAt={at(5 * 86_400_000)}
          now={NOW}
        />
      </I18nextProvider>,
    )
    expect(screen.getByText('Expires in 5 days')).toBeTruthy()
    rerender(
      <I18nextProvider i18n={i18n}>
        <GitCredentialStatus
          kind="static_token"
          expiresAt={at(20 * 3_600_000)}
          now={NOW}
        />
      </I18nextProvider>,
    )
    expect(screen.getByText('Expires in 20 hours')).toBeTruthy()
    rerender(
      <I18nextProvider i18n={i18n}>
        <GitCredentialStatus kind="static_token" now={NOW} />
      </I18nextProvider>,
    )
    expect(screen.getByText('No expiry reported')).toBeTruthy()
  })
})
