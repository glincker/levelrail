import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { CacheBucket } from '../../queries/cdnCache'
import { CdnI18n } from '../../test/cdnI18n'
import { CachesPage } from './CachesPage'
import { HitRateChart } from './HitRateChart'
import { PurgeHistoryTable } from './PurgeHistoryTable'
import { PurgePanel } from './PurgePanel'
import { PurgeScopePicker } from './PurgeScopePicker'
import { RankedBarList } from './RankedBarList'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: ReactNode }) => <a href="#x">{children}</a>,
}))

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function mockFetch(routes: Record<string, () => Response>) {
  const fn = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : input.url
    const key = `${init?.method ?? 'GET'} ${url}`
    const hit = Object.entries(routes).find(([k]) => key.startsWith(k))
    return hit
      ? Promise.resolve(hit[1]())
      : Promise.reject(new Error(`unexpected fetch: ${key}`))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <CdnI18n>{node}</CdnI18n>
    </QueryClientProvider>,
  )
}

const bucket = (minute: number, hits: number, misses: number): CacheBucket => ({
  minute,
  hits,
  misses,
  hit_bytes: 0,
  miss_bytes: 0,
})

afterEach(() => vi.unstubAllGlobals())

describe('HitRateChart', () => {
  it('explains an empty cache instead of drawing an empty chart', () => {
    render(
      <CdnI18n>
        <HitRateChart series={[bucket(1, 0, 0)]} />
      </CdnI18n>,
    )
    expect(screen.getByText(/No cache activity yet/)).toBeTruthy()
    expect(screen.queryByRole('img')).toBeNull()
  })

  it('describes the totals for assistive tech', () => {
    render(
      <CdnI18n>
        <HitRateChart series={[bucket(1, 8, 2), bucket(2, 5, 0)]} />
      </CdnI18n>,
    )
    expect(
      screen.getByRole('img', { name: /13 cache hits and 2 misses/ }),
    ).toBeTruthy()
  })
})

describe('RankedBarList', () => {
  it('renders each row with its share', () => {
    render(
      <RankedBarList
        ariaLabel="by server"
        items={[
          {
            id: 'a',
            label: 'edge-1',
            detail: 'fsn1',
            share: 0.7,
            valueLabel: '70.0%',
          },
          { id: 'b', label: 'edge-2', share: 0.3, valueLabel: '30.0%' },
        ]}
      />,
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('70.0%')).toBeTruthy()
  })
})

describe('PurgeScopePicker', () => {
  it('is a radio group and reports the chosen scope', () => {
    const onChange = vi.fn()
    render(
      <CdnI18n>
        <PurgeScopePicker value="url" onChange={onChange} />
      </CdnI18n>,
    )
    expect(screen.getAllByRole('radio')).toHaveLength(4)
    fireEvent.click(screen.getByRole('radio', { name: /Cache tag/ }))
    expect(onChange).toHaveBeenCalledWith('tag')
  })
})

describe('PurgePanel', () => {
  it('purges a prefix straight away', async () => {
    const fn = mockFetch({
      'POST /api/v1/apps/web/cache/purge': () =>
        json({ removed: 3, domains: [] }),
    })
    wrap(<PurgePanel app="web" />)
    fireEvent.click(screen.getByRole('radio', { name: /Path prefix/ }))
    fireEvent.change(screen.getByLabelText('Path prefix'), {
      target: { value: '/static/' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Purge' }))
    await waitFor(() =>
      expect(screen.getByText(/Removed 3 cached/)).toBeTruthy(),
    )
    const sent = fn.mock.calls[0]?.[1]?.body as string
    const body = JSON.parse(sent) as unknown
    expect(body).toEqual({ scope: 'prefix', value: '/static/' })
  })

  it('makes purging everything wait for a typed confirmation', async () => {
    const fn = mockFetch({
      'POST /api/v1/apps/web/cache/purge': () =>
        json({ removed: 9, domains: [] }),
    })
    wrap(<PurgePanel app="web" />)
    fireEvent.click(screen.getByRole('radio', { name: /^Everything/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Purge' }))
    expect(fn).not.toHaveBeenCalled()
    expect(await screen.findByText('Purge everything for web?')).toBeTruthy()
    const confirm = screen.getByRole('button', { name: 'Purge everything' })
    expect((confirm as HTMLButtonElement).disabled).toBe(true)
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'web' } })
    fireEvent.click(confirm)
    await waitFor(() => expect(fn).toHaveBeenCalledTimes(1))
  })

  it('shows the server error instead of failing silently', async () => {
    mockFetch({
      'POST /api/v1/apps/web/cache/purge': () =>
        json({ error: 'value must be one cache tag' }, 400),
    })
    wrap(<PurgePanel app="web" />)
    fireEvent.click(screen.getByRole('radio', { name: /Cache tag/ }))
    fireEvent.change(screen.getByLabelText('Cache tag'), {
      target: { value: 'x' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Purge' }))
    expect(await screen.findByText('value must be one cache tag')).toBeTruthy()
  })
})

describe('PurgeHistoryTable', () => {
  it('has an empty state that says what fills it', async () => {
    mockFetch({
      'GET /api/v1/apps/web/cache/purges': () => json({ purges: [] }),
    })
    wrap(<PurgeHistoryTable app="web" />)
    expect(await screen.findByText('No purges yet')).toBeTruthy()
  })

  it('lists purges with who ran them', async () => {
    mockFetch({
      'GET /api/v1/apps/web/cache/purges': () =>
        json({
          purges: [
            {
              domain: 'a.test',
              scope: 'tag',
              value: 'blog',
              removed: 4,
              actor: 'root',
              at: '2026-10-17T10:00:00Z',
            },
          ],
        }),
    })
    wrap(<PurgeHistoryTable app="web" />)
    expect(await screen.findByText('root')).toBeTruthy()
    expect(screen.getByText('blog')).toBeTruthy()
  })

  it('shows an error state', async () => {
    mockFetch({
      'GET /api/v1/apps/web/cache/purges': () => json({ error: 'boom' }, 500),
    })
    wrap(<PurgeHistoryTable app="web" />)
    expect(await screen.findByText('boom')).toBeTruthy()
  })
})

describe('CachesPage', () => {
  const limits = {
    max_bytes: 1000,
    used_bytes: 100,
    max_object_bytes: 10,
    tag_header: 'Cache-Tag',
    location: 'control-plane-ingress',
  }
  const zero = {
    hits: 0,
    misses: 0,
    stale: 0,
    bypasses: 0,
    hit_rate: 0,
    hit_bytes: 0,
    origin_bytes: 0,
    entries: 0,
    bytes: 0,
  }

  it('offers a retry when the cache cannot be read', async () => {
    mockFetch({
      'GET /api/v1/system/cache': () => json({ error: 'down' }, 500),
    })
    wrap(<CachesPage />)
    expect(await screen.findByText('down')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy()
  })

  it('ranks apps and explains where entries live', async () => {
    mockFetch({
      'GET /api/v1/system/cache': () =>
        json({
          limits,
          totals: zero,
          series: [],
          apps: [
            {
              app: 'web',
              domains: 1,
              enabled_domains: 1,
              requests: 10,
              ...zero,
              hits: 8,
              misses: 2,
              hit_rate: 0.8,
              reasons: [],
            },
          ],
        }),
    })
    wrap(<CachesPage />)
    expect(await screen.findByText('web')).toBeTruthy()
    expect(screen.getByText('Where entries live')).toBeTruthy()
    expect(screen.getByText('80.0%')).toBeTruthy()
  })

  it('says why an app with no hits is not cached and offers the Next.js preset', async () => {
    const vary = (detail: string) => ({
      code: 'vary_header',
      detail,
      count: 40,
      domains: ['web.test'],
    })
    const fn = mockFetch({
      'GET /api/v1/system/cache': () =>
        json({
          limits,
          totals: zero,
          series: [],
          apps: [
            {
              app: 'web',
              domains: 1,
              enabled_domains: 1,
              requests: 40,
              ...zero,
              misses: 40,
              reasons: [vary('Rsc'), vary('Next-Router-State-Tree')],
              suggest_preset: 'nextjs',
            },
          ],
        }),
      'GET /api/v1/apps/web/domains/web.test/policies': () =>
        json({ policy: { cache: { enabled: true, rules: [] } } }),
      'PUT /api/v1/apps/web/domains/web.test/cache': () => json({}),
    })
    wrap(<CachesPage />)
    expect(await screen.findAllByText('The app varies on Rsc')).toHaveLength(2)
    expect(screen.getByText(/looks like a Next.js app/)).toBeTruthy()
    fireEvent.click(
      screen.getByRole('button', { name: 'Apply Next.js preset' }),
    )
    await waitFor(() =>
      expect(fn.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(
        true,
      ),
    )
    const put = fn.mock.calls.find(([, init]) => init?.method === 'PUT')
    const spec = JSON.parse(put?.[1]?.body as string) as {
      enabled: boolean
      rules: { vary?: string[] }[]
    }
    expect(spec.enabled).toBe(true)
    expect(spec.rules.at(-1)?.vary).toContain('rsc')
  })

  it('adds a plain Vary header from the reason row', async () => {
    const fn = mockFetch({
      'GET /api/v1/system/cache': () =>
        json({
          limits,
          totals: zero,
          series: [],
          apps: [
            {
              app: 'web',
              domains: 1,
              enabled_domains: 1,
              requests: 5,
              ...zero,
              misses: 5,
              reasons: [
                {
                  code: 'vary_header',
                  detail: 'Accept-Language',
                  count: 5,
                  domains: ['web.test'],
                },
              ],
            },
          ],
        }),
      'GET /api/v1/apps/web/domains/web.test/policies': () =>
        json({
          policy: {
            cache: {
              enabled: true,
              rules: [{ match: { kind: 'prefix', path: '/' }, ttl_seconds: 5 }],
            },
          },
        }),
      'PUT /api/v1/apps/web/domains/web.test/cache': () => json({}),
    })
    wrap(<CachesPage />)
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Add Accept-Language to Vary',
      }),
    )
    await waitFor(() =>
      expect(fn.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(
        true,
      ),
    )
    const put = fn.mock.calls.find(([, init]) => init?.method === 'PUT')
    expect(put?.[1]?.body as string).toContain('"vary":["Accept-Language"]')
  })
})
