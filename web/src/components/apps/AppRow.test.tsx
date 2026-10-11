import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppRow } from '../AppRow'
import { withLimit } from '../../queries/fleetTraffic'
import type { AppListEntry } from '../../types/appDetail'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

function makeApp(name: string, over: Partial<AppListEntry> = {}): AppListEntry {
  return {
    name,
    image: 'nginx:1.27',
    port: 80,
    domains: ['web.example.com'],
    environment_name: 'production',
    status: { label: 'Healthy', variant: 'success' },
    ...over,
  } as AppListEntry
}

function batch(names: string[], hasTraffic: boolean, p95: number, err: number) {
  return names.map((name) => ({
    name,
    has_traffic: hasTraffic,
    p95_ms: p95,
    error_rate_5xx: err,
    rate_per_sec: 2,
    spark: [0, 1, 2, 3, 4, 5],
    last_deploy_at: new Date().toISOString(),
  }))
}

const urls: string[] = []

function stubFetch(rows: ReturnType<typeof batch>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      urls.push(String(url))
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(rows),
      } as unknown as Response)
    }),
  )
}

function renderRows(apps: AppListEntry[]) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      {apps.map((a) => (
        <AppRow key={a.name} app={a} />
      ))}
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  urls.length = 0
})

describe('AppRow', () => {
  it('renders meta chips and traffic numerals, warning tone over the p95 threshold', async () => {
    stubFetch(batch(['web'], true, 800, 0.002))
    renderRows([makeApp('web')])
    expect(screen.getByText('production')).toBeInTheDocument()
    expect(screen.getByText('web.example.com')).toBeInTheDocument()
    expect(screen.getByText('Healthy').className).toContain('text-tone-success')
    expect(screen.getByText('nginx:1.27')).toBeInTheDocument()
    const p95 = await screen.findByText('800 ms')
    expect(p95.className).toContain('tone-warning')
    expect(screen.getByText('0.2%').className).toContain('muted')
    expect(
      screen.getByRole('img', { name: /Request rate for web/ }),
    ).toBeInTheDocument()
  })

  it('turns errors red past the critical threshold', async () => {
    stubFetch(batch(['web'], true, 100, 0.09))
    renderRows([makeApp('web')])
    expect((await screen.findByText('9.0%')).className).toContain('tone-danger')
  })

  it('shows placeholders without traffic', async () => {
    stubFetch(batch(['web'], false, 0, 0))
    renderRows([makeApp('web')])
    expect(await screen.findByText('No traffic')).toBeInTheDocument()
    expect(screen.queryByText(/ ms$/)).not.toBeInTheDocument()
  })

  it('issues one batched metrics request for many rows', async () => {
    const apps = Array.from({ length: 10 }, (_, i) => makeApp(`app-${i}`))
    stubFetch(
      batch(
        apps.map((a) => a.name),
        true,
        100,
        0,
      ),
    )
    renderRows(apps)
    await waitFor(() => {
      expect(screen.getAllByText('100 ms')).toHaveLength(10)
    })
    expect(urls).toEqual(['/api/v1/apps-metrics'])
  })
})

describe('withLimit', () => {
  it('runs queued tasks after earlier ones finish', async () => {
    const order: number[] = []
    await Promise.all(
      [1, 2, 3, 4, 5, 6].map((n) =>
        withLimit(async () => {
          await new Promise((r) => setTimeout(r, 2))
          order.push(n)
        }),
      ),
    )
    expect(order).toHaveLength(6)
  })
})
