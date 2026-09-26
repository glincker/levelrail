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

function series(hasTraffic: boolean, p95: number, err: number) {
  return {
    app: 'x',
    summary: {
      has_traffic: hasTraffic,
      p95_ms: p95,
      error_rate_5xx: err,
      rate_per_sec: 2,
    },
    points: Array.from({ length: 6 }, (_, i) => ({
      timestamp: `t${i}`,
      rate_per_sec: i,
    })),
  }
}

let inflight = 0
let maxInflight = 0
const urls: string[] = []

function stubFetch(traffic: ReturnType<typeof series>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      urls.push(String(url))
      inflight += 1
      maxInflight = Math.max(maxInflight, inflight)
      await new Promise((r) => setTimeout(r, 5))
      inflight -= 1
      const body = String(url).includes('/requests')
        ? traffic
        : [
            {
              id: '1',
              status: 'succeeded',
              started_at: new Date().toISOString(),
            },
          ]
      return {
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      } as unknown as Response
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
  inflight = 0
  maxInflight = 0
  urls.length = 0
})

describe('AppRow', () => {
  it('renders meta chips and traffic numerals, amber over the p95 threshold', async () => {
    stubFetch(series(true, 800, 0.002))
    renderRows([makeApp('web')])
    expect(screen.getByText('production')).toBeInTheDocument()
    expect(screen.getByText('web.example.com')).toBeInTheDocument()
    const p95 = await screen.findByText('800 ms')
    expect(p95.className).toContain('amber')
    expect(screen.getByText('0.2%').className).toContain('muted')
    expect(
      screen.getByRole('img', { name: /Request rate for web/ }),
    ).toBeInTheDocument()
  })

  it('turns errors red past the critical threshold', async () => {
    stubFetch(series(true, 100, 0.09))
    renderRows([makeApp('web')])
    expect((await screen.findByText('9.0%')).className).toContain('destructive')
  })

  it('shows placeholders without traffic', async () => {
    stubFetch(series(false, 0, 0))
    renderRows([makeApp('web')])
    expect(await screen.findByText('No traffic')).toBeInTheDocument()
    expect(screen.queryByText(/ ms$/)).not.toBeInTheDocument()
  })

  it('never fires the fleet-wide summary or more than four requests at once', async () => {
    stubFetch(series(true, 100, 0))
    const apps = Array.from({ length: 10 }, (_, i) => makeApp(`app-${i}`))
    renderRows(apps)
    await waitFor(() => {
      expect(urls.filter((u) => u.includes('/requests'))).toHaveLength(10)
      expect(urls.filter((u) => u.includes('deploy-attempts'))).toHaveLength(10)
    })
    expect(maxInflight).toBeLessThanOrEqual(4)
    expect(urls.some((u) => u.includes('apps-summary'))).toBe(false)
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
