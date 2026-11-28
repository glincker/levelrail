import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'
import tokenListEn from '../../locales/en/tokenList.json'
import tokensNoticesEn from '../../locales/en/tokensNotices.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { TokensExplorer } from './TokensExplorer'
import { TokensNotices } from './TokensNotices'
import type { StatusFilter } from './TokensToolbar'
import type { ServedToken } from '../../types/tokenList'

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('../../hooks/useTokenExpiryPolicy', () => ({
  useTokenExpiryPolicy: () => ({ soonDays: 14, windows: null, grace: null }),
}))
vi.mock('../RevokeTokenDialog', () => ({
  RevokeTokenDialog: () => <button type="button">row-revoke</button>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['tokenList', 'common', 'settings', 'tokensNotices'],
  defaultNS: 'common',
  resources: {
    en: {
      tokenList: tokenListEn,
      common: commonEn,
      settings: settingsEn,
      tokensNotices: tokensNoticesEn,
    },
  },
})

const PAST = '2026-01-01T00:00:00Z'
function tok(
  i: number,
  name: string,
  over: Partial<ServedToken> = {},
): ServedToken {
  return {
    id: `id-${i}`,
    name,
    abilities: ['root'],
    created_at: PAST,
    status: 'active',
    ...over,
  }
}

interface Server {
  tokens: ServedToken[]
  sessionStatus: number
  bulk: Record<string, unknown>[]
}
let server: Server

function counts(items: ServedToken[]) {
  const c = { active: 0, expiring: 0, expired: 0, revoked: 0 }
  for (const x of items) c[x.status]++
  return c
}

function respond(url: URL, init?: RequestInit): unknown {
  if (url.pathname === '/api/v1/auth/session') {
    return { status: server.sessionStatus, body: { username: 'me' } }
  }
  if (url.pathname === '/api/v1/tokens/bulk-revoke') {
    const body = JSON.parse(init?.body as string) as Record<string, unknown>
    server.bulk.push(body)
    const n = Array.isArray(body.ids)
      ? body.ids.length
      : server.tokens.filter((x) => x.status === 'active').length
    return {
      status: 200,
      body: {
        dry_run: Boolean(body.dry_run),
        count: n,
        sample: server.tokens.slice(0, 3).map((x) => x.name),
        revoked: body.dry_run ? 0 : n,
        max: 0,
      },
    }
  }
  const sp = url.searchParams
  let items = server.tokens.filter(
    (x) => !sp.get('status') || x.status === sp.get('status'),
  )
  if (sp.get('name')) items = items.filter((x) => x.name === sp.get('name'))
  if (sp.get('ability')) {
    items = items.filter((x) =>
      x.abilities.includes(sp.get('ability') as never),
    )
  }
  const base = { total: items.length, counts_by_status: counts(items) }
  if (sp.get('groups') === 'name') {
    const by = new Map<string, ServedToken[]>()
    for (const x of items) by.set(x.name, [...(by.get(x.name) ?? []), x])
    return {
      status: 200,
      body: {
        ...base,
        groups: [...by].map(([name, g]) => ({
          name,
          count: g.length,
          active: g.filter((x) => x.status === 'active').length,
          expiring: 0,
          expired: 0,
          oldest_created: PAST,
        })),
      },
    }
  }
  const limit = Number(sp.get('limit') ?? 100)
  const from = Number(sp.get('cursor') ?? 0)
  const end = Math.min(from + limit, items.length)
  return {
    status: 200,
    body: {
      ...base,
      items: items.slice(from, end),
      ...(end < items.length ? { next_cursor: String(end) } : {}),
    },
  }
}

function Harness() {
  const [status, setStatus] = useState<StatusFilter>('active')
  return <TokensExplorer status={status} onStatus={setStatus} />
}

function renderExplorer() {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <Harness />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

beforeEach(() => {
  server = { tokens: [], sessionStatus: 200, bulk: [] }
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      const r = respond(new URL(input, 'http://x'), init) as {
        status: number
        body: unknown
      }
      return Promise.resolve({
        ok: r.status < 400,
        status: r.status,
        json: () => Promise.resolve(r.body),
      } as unknown as Response)
    }),
  )
})
afterEach(() => {
  vi.unstubAllGlobals()
})

const emergency = (n: number) =>
  Array.from({ length: n }, (_, i) => tok(i, 'emergency (host)'))

describe('grouped view', () => {
  it('collapses big groups, keeps small ones open, loads rows on expand', async () => {
    server.tokens = [...emergency(12), tok(100, 'ci'), tok(101, 'ci')]
    renderExplorer()
    const big = await screen.findByRole('button', {
      name: 'emergency (host), 12 tokens',
    })
    expect(big).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.getByRole('button', { name: 'ci, 2 tokens' }),
    ).toHaveAttribute('aria-expanded', 'true')
    expect(await screen.findAllByRole('listitem')).toHaveLength(2)
    expect(
      screen.getByText(/12 live root tokens named emergency/),
    ).toBeVisible()
    await userEvent.click(big)
    expect(big).toHaveAttribute('aria-expanded', 'true')
    await waitFor(() => {
      expect(screen.getAllByRole('listitem')).toHaveLength(14)
    })
  })

  it('selects a group and revokes it through dry run then confirm_count', async () => {
    server.tokens = emergency(12)
    renderExplorer()
    await userEvent.click(
      await screen.findByRole('checkbox', {
        name: 'Select all tokens named emergency (host)',
      }),
    )
    expect(
      screen.getByText('All 12 tokens matching this filter are selected'),
    ).toBeVisible()
    await userEvent.click(
      screen.getByRole('button', { name: 'Revoke selected' }),
    )
    expect(await screen.findByText('12 tokens will be revoked')).toBeVisible()
    expect(server.bulk[0]).toMatchObject({ dry_run: true })
    expect(screen.getByText('12 root tokens are included')).toBeVisible()
    const confirm = screen.getByRole('button', { name: 'Revoke 12 tokens' })
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText('Number of tokens'), '12')
    await userEvent.click(confirm)
    expect(await screen.findByText('Revoked 12, skipped 0')).toBeVisible()
    expect(server.bulk[1]).toMatchObject({
      confirm_count: 12,
      filter: { name: 'emergency (host)', status: 'active' },
    })
  })
})

describe('flat view', () => {
  const distinct = (n: number) =>
    Array.from({ length: n }, (_, i) => tok(i, `token-${i}`))

  it('pages 1000 tokens without mounting them all, then selects all matching', async () => {
    server.tokens = distinct(1000)
    renderExplorer()
    await waitFor(() => {
      expect(screen.getAllByRole('listitem')).toHaveLength(100)
    })
    expect(screen.getByRole('button', { name: 'Load more' })).toBeVisible()
    await userEvent.click(
      screen.getByRole('checkbox', { name: 'Select all 100 loaded tokens' }),
    )
    expect(screen.getAllByText('100 tokens selected').length).toBeGreaterThan(0)
    await userEvent.click(
      screen.getByRole('button', {
        name: 'Select all 1000 tokens matching this filter',
      }),
    )
    expect(
      screen.getByText('All 1000 tokens matching this filter are selected'),
    ).toBeVisible()
    await userEvent.click(
      screen.getByRole('button', { name: 'Revoke selected' }),
    )
    await screen.findByText('1000 tokens will be revoked')
    expect(server.bulk[0]).toMatchObject({
      dry_run: true,
      filter: { status: 'active' },
    })
  })

  it('selects a shift range and shows the bulk bar only with a selection', async () => {
    server.tokens = distinct(6)
    renderExplorer()
    const box = async (n: number) =>
      screen.findByRole('checkbox', { name: `Select token-${n}` })
    expect(screen.queryByRole('region', { name: 'Bulk actions' })).toBeNull()
    await userEvent.click(await box(1))
    fireEvent.click(await box(4), { shiftKey: true })
    expect(screen.getAllByText('4 tokens selected').length).toBeGreaterThan(0)
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }))
    expect(screen.queryByRole('region', { name: 'Bulk actions' })).toBeNull()
  })

  it('never shows an expired token as Active', async () => {
    server.tokens = [
      tok(1, 'old', { status: 'expired', expires_at: PAST }),
      tok(2, 'live'),
    ]
    renderExplorer()
    await userEvent.click(await screen.findByRole('radio', { name: /^All/ }))
    const row = (await screen.findByText('old')).closest('li') as HTMLElement
    expect(row).toHaveAttribute('data-status', 'expired')
    expect(within(row).getByText('Expired')).toBeVisible()
    expect(within(row).queryByText('Active')).toBeNull()
  })

  it('disables bulk revoke for an API token session', async () => {
    server.sessionStatus = 401
    server.tokens = distinct(3)
    renderExplorer()
    await userEvent.click(
      await screen.findByRole('checkbox', { name: 'Select token-1' }),
    )
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Revoke selected' }),
      ).toBeDisabled()
    })
    expect(
      screen.getByText(/needs a signed-in dashboard session/),
    ).toBeVisible()
  })
})

describe('expiry banner', () => {
  it('uses the server count and opens the expiring list', async () => {
    const open = vi.fn()
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ alert_days: 0, items: [] }),
        } as unknown as Response),
      ),
    )
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <TokensNotices
            expiringSoon={3}
            soonDays={14}
            longLivedRoot={0}
            onReviewExpiring={open}
          />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(
      screen.getByText('3 active tokens expire within 14 days', {
        exact: false,
      }),
    ).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Show them' }))
    expect(open).toHaveBeenCalledOnce()
  })
})
