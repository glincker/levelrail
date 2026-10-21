import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GitRepoListPicker } from './GitRepoListPicker'
import commonEn from '../locales/en/common.json'
import gitReposEn from '../locales/en/gitRepos.json'
import type { GitRepoItem } from '../types/gitRepos'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'gitRepos'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, gitRepos: gitReposEn } },
  interpolation: { escapeValue: false },
})

function item(
  owner: string,
  name: string,
  extra: Partial<GitRepoItem> = {},
): GitRepoItem {
  return {
    key: `${owner}/${name}`,
    full_name: `${owner}/${name}`,
    name,
    owner,
    private: false,
    archived: false,
    fork: false,
    default_branch: 'main',
    clone_url: `https://github.com/${owner}/${name}.git`,
    ...extra,
  }
}

function page(items: GitRepoItem[], extra: Record<string, unknown> = {}) {
  return {
    provider: 'github',
    items,
    total: items.length,
    loaded: items.length,
    limit: 100,
    offset: 0,
    next_offset: null,
    has_more: false,
    truncated: false,
    cap: 10000,
    cached_at: '2026-10-20T10:00:00Z',
    stale: false,
    refreshing: false,
    owners: [],
    ...extra,
  }
}

function json(body: unknown, status = 200): Promise<Response> {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function mockFetch(handler: (url: string) => Promise<Response>) {
  const fn = vi.fn((input: RequestInfo | URL) => handler(urlOf(input)))
  vi.stubGlobal('fetch', fn)
  return fn
}

function renderPicker(onSelect = vi.fn()): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <GitRepoListPicker
          provider="github"
          enabled
          selectedKey=""
          runningRepoByUrl={new Map()}
          searchInputId="picker-search"
          onSelect={onSelect}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GitRepoListPicker', () => {
  it('shows the owner on same-name repositories from different owners', async () => {
    mockFetch(() => json(page([item('acme', 'api'), item('octocat', 'api')])))
    renderPicker()

    const cards = await screen.findAllByRole('button', { name: /api/ })
    expect(cards.map((c) => c.textContent)).toEqual([
      expect.stringContaining('acme/api'),
      expect.stringContaining('octocat/api'),
    ])
  })

  it('says how many are shown and asks to narrow when the list is longer than the page', async () => {
    mockFetch(() =>
      json(
        page([item('acme', 'one'), item('acme', 'two')], {
          total: 2300,
          loaded: 2300,
          next_offset: 2,
          has_more: true,
        }),
      ),
    )
    renderPicker()

    expect(await screen.findByText(/Showing 2 of 2300\./)).toHaveTextContent(
      'Search or pick an owner to narrow the list.',
    )
    expect(
      screen.getByRole('button', { name: 'Show more' }),
    ).toBeInTheDocument()
  })

  it('warns when the account has more repositories than the cap', async () => {
    mockFetch(() =>
      json(page([item('acme', 'one')], { truncated: true, cap: 10000 })),
    )
    renderPicker()

    expect(
      await screen.findByText(/more than the 10000 repositories listed here/),
    ).toBeInTheDocument()
  })

  it('searches on the server instead of filtering what it already has', async () => {
    const fetchMock = mockFetch((url) =>
      url.includes('search=zebra')
        ? json(page([item('acme', 'zebra')]))
        : json(
            page([item('acme', 'one')], {
              total: 5000,
              loaded: 5000,
              has_more: true,
              next_offset: 1,
            }),
          ),
    )
    renderPicker()
    await screen.findByRole('button', { name: /acme\/one/ })

    fireEvent.change(screen.getByLabelText('Search repositories...'), {
      target: { value: 'zebra' },
    })
    expect(
      await screen.findByRole('button', { name: /acme\/zebra/ }),
    ).toBeInTheDocument()
    expect(
      fetchMock.mock.calls.map((c) => urlOf(c[0] as RequestInfo)),
    ).toContainEqual(expect.stringContaining('search=zebra'))
  })

  it('sends the archived and fork toggles to the server', async () => {
    const fetchMock = mockFetch(() => json(page([item('acme', 'one')])))
    renderPicker()
    await screen.findByRole('button', { name: /acme\/one/ })

    fireEvent.click(screen.getByRole('button', { name: 'Hide archived' }))
    fireEvent.click(screen.getByRole('button', { name: 'Hide forks' }))
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((c) => urlOf(c[0] as RequestInfo))
      expect(
        urls.some(
          (u) => u.includes('archived=exclude') && u.includes('forks=exclude'),
        ),
      ).toBe(true)
    })
  })

  it('refresh asks the server to re-list from the provider', async () => {
    const fetchMock = mockFetch(() => json(page([item('acme', 'one')])))
    renderPicker()
    await screen.findByRole('button', { name: /acme\/one/ })

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((c) => urlOf(c[0] as RequestInfo))
      expect(urls.some((u) => u.includes('refresh=1'))).toBe(true)
    })
  })

  it('shows the next check time when rate limited with nothing saved, and retries on demand', async () => {
    let calls = 0
    mockFetch(() => {
      calls += 1
      return calls === 1
        ? json(
            {
              error: 'rate limited',
              code: 'rate_limited',
              next_check_at: '2026-10-20T10:05:00Z',
            },
            429,
          )
        : json(page([item('acme', 'one')]))
    })
    renderPicker()

    expect(
      await screen.findByText(
        /rate limiting this control plane and there is no saved list yet/,
      ),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(
      await screen.findByRole('button', { name: /acme\/one/ }),
    ).toBeInTheDocument()
  })

  it('shows the saved list with a rate limit notice when a refresh was blocked', async () => {
    mockFetch(() =>
      json(
        page([item('acme', 'one')], {
          stale: true,
          rate_limit: {
            next_check_at: '2026-10-20T10:05:00Z',
            retry_after_seconds: 60,
          },
        }),
      ),
    )
    renderPicker()

    expect(
      await screen.findByRole('button', { name: /acme\/one/ }),
    ).toBeInTheDocument()
    expect(screen.getByText(/this is the saved list/)).toBeInTheDocument()
    expect(
      screen.getByText(/Saved list, refreshing in the background\./),
    ).toBeInTheDocument()
  })

  it('marks archived and fork repositories', async () => {
    mockFetch(() =>
      json(
        page([
          item('acme', 'old', { archived: true }),
          item('acme', 'copy', { fork: true }),
        ]),
      ),
    )
    renderPicker()

    expect(await screen.findByText('Archived')).toBeInTheDocument()
    expect(screen.getByText('Fork')).toBeInTheDocument()
  })

  it('selecting a card reports the normalized option', async () => {
    mockFetch(() => json(page([item('acme', 'one')])))
    const onSelect = vi.fn()
    renderPicker(onSelect)

    fireEvent.click(await screen.findByRole('button', { name: /acme\/one/ }))
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({
        cloneUrl: 'https://github.com/acme/one.git',
        providerRef: { kind: 'github', owner: 'acme', repo: 'one' },
      }),
    )
  })

  it('shows the account on every row and filters by account when several are connected', async () => {
    const ann = {
      connection_id: 'gc_a',
      connection_label: 'ann @ gitlab.com',
      account: 'ann',
    }
    const bob = {
      connection_id: 'gc_b',
      connection_label: 'Work',
      account: 'bob',
    }
    const facets = [
      {
        connection_id: 'gc_a',
        provider: 'gitlab',
        label: 'ann @ gitlab.com',
        account: 'ann',
        loaded: 1,
        failed: false,
      },
      {
        connection_id: 'gc_b',
        provider: 'gitlab',
        label: 'Work',
        account: 'bob',
        loaded: 1,
        failed: false,
      },
    ]
    const fetchMock = mockFetch((url) => {
      if (url.includes('connection=gc_b')) {
        return json(page([item('corp', 'web', bob)], { connections: facets }))
      }
      return json(
        page([item('ann', 'web', ann), item('corp', 'web', bob)], {
          connections: facets,
        }),
      )
    })
    renderPicker()

    await screen.findByText('Account: ann')
    expect(screen.getByText('Account: bob')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: /web/ })).toHaveLength(2)

    fireEvent.click(screen.getByRole('button', { name: 'bob (1)' }))
    await waitFor(() => {
      expect(screen.queryByText('Account: ann')).not.toBeInTheDocument()
    })
    expect(
      fetchMock.mock.calls.some(([input]) =>
        urlOf(input).includes('connection=gc_b'),
      ),
    ).toBe(true)
  })
})
