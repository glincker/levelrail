import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { Suspense } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GitConnectionsCard } from './GitConnectionsCard'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'
import type { GitProviderConnection } from '../../types/gitConnections'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function conn(
  id: string,
  extra: Partial<GitProviderConnection> = {},
): GitProviderConnection {
  return {
    id,
    provider: 'gitlab',
    label: `Account ${id}`,
    host: 'gitlab.com',
    instance_url: 'https://gitlab.com',
    client_id: 'cid',
    account_name: id,
    is_default: false,
    authorized: true,
    created_at: '2026-10-20T10:00:00Z',
    source_count: 0,
    connection: { status: 'ok' },
    authorize_path: `/api/v1/gitlab-app/connect?connection_id=${id}`,
    ...extra,
  }
}

let connections: GitProviderConnection[]
let calls: { method: string; url: string; body?: string }[]

beforeEach(() => {
  calls = []
  connections = [
    conn('ann', { is_default: true, source_count: 2 }),
    conn('bob', {
      host: 'git.corp.example',
      connection: { status: 'needs_reauthorize', detail: 'token revoked' },
    }),
    conn('cat', { authorized: false }),
  ]
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.toString()
            : input.url
      const method = init?.method ?? 'GET'
      calls.push({ method, url, body: init?.body as string | undefined })
      const body =
        method === 'DELETE'
          ? { disconnected: true, token_revoked: true, note: 'revoked' }
          : method === 'GET'
            ? connections
            : conn('new')
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderCard() {
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <Suspense fallback={<div>loading</div>}>
          <GitConnectionsCard provider="gitlab" />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('GitConnectionsCard', () => {
  it('lists every account with its host, state and sources', async () => {
    renderCard()
    const rows = await screen.findAllByRole('listitem')
    expect(rows).toHaveLength(3)
    expect(within(rows[0]!).getByText('Default')).toBeInTheDocument()
    expect(within(rows[0]!).getByText(/2 git sources/)).toBeInTheDocument()
    expect(within(rows[1]!).getByText('Needs reconnect')).toBeInTheDocument()
    expect(within(rows[1]!).getByText('git.corp.example')).toBeInTheDocument()
    expect(within(rows[2]!).getByText('Not authorized')).toBeInTheDocument()
    expect(
      within(rows[2]!).getByRole('button', { name: 'Authorize' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Add another account' }),
    ).toBeInTheDocument()
  })

  it('reconnects through the connection authorize path', async () => {
    const assign = vi.fn()
    vi.stubGlobal('location', { ...window.location, assign })
    renderCard()
    const rows = await screen.findAllByRole('listitem')
    await userEvent.click(
      within(rows[1]!).getByRole('button', { name: 'Reconnect' }),
    )
    expect(assign).toHaveBeenCalledWith(
      '/api/v1/gitlab-app/connect?connection_id=bob',
    )
  })

  it('renames one account and makes another the default', async () => {
    renderCard()
    const rows = await screen.findAllByRole('listitem')
    await userEvent.click(
      within(rows[1]!).getByRole('button', { name: 'Rename' }),
    )
    const input = screen.getByLabelText('Connection label')
    await userEvent.clear(input)
    await userEvent.type(input, 'Work GitLab')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(calls).toContainEqual({
        method: 'PATCH',
        url: '/api/v1/git-connections/bob',
        body: JSON.stringify({ label: 'Work GitLab' }),
      })
    })

    const fresh = await screen.findAllByRole('listitem')
    await userEvent.click(
      within(fresh[1]!).getByRole('button', { name: 'Set default' }),
    )
    await waitFor(() => {
      expect(
        calls.some(
          (c) =>
            c.method === 'POST' &&
            c.url === '/api/v1/git-connections/bob/default',
        ),
      ).toBe(true)
    })
  })

  it('disconnects only the chosen account after telling you what happens to its sources', async () => {
    renderCard()
    const rows = await screen.findAllByRole('listitem')
    await userEvent.click(
      within(rows[0]!).getByRole('button', { name: 'Disconnect' }),
    )
    expect(
      await screen.findByText(/2 git sources use this account/),
    ).toBeInTheDocument()
    const dialog = screen.getByRole('dialog')
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Disconnect' }),
    )
    await waitFor(() => {
      expect(
        calls.some(
          (c) =>
            c.method === 'DELETE' && c.url === '/api/v1/git-connections/ann',
        ),
      ).toBe(true)
    })
    expect(calls.filter((c) => c.method === 'DELETE')).toHaveLength(1)
  })

  it('adds another account by reusing an existing OAuth application', async () => {
    const assign = vi.fn()
    vi.stubGlobal('location', { ...window.location, assign })
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Add another account' }),
    )
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('combobox'))
    await userEvent.click(
      await screen.findByRole('option', {
        name: /Reuse the application of Account ann/,
      }),
    )
    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Save and authorize' }),
    )
    await waitFor(() => {
      expect(calls).toContainEqual({
        method: 'POST',
        url: '/api/v1/git-connections',
        body: JSON.stringify({
          provider: 'gitlab',
          reuse_connection_id: 'ann',
        }),
      })
    })
    await waitFor(() => {
      expect(assign).toHaveBeenCalledWith(
        '/api/v1/gitlab-app/connect?connection_id=new',
      )
    })
  })

  it('offers a first-account empty state with a next action', async () => {
    connections = []
    renderCard()
    expect(await screen.findByText('No account connected')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Add an account' }),
    ).toBeInTheDocument()
  })
})
