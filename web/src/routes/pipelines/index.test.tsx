import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Route } from './index'
import type {
  PipelineRunRow,
  PipelineRunRowsPage,
  PipelineSummary,
} from '../../types/pipelineOverview'
import type { Brand } from '../../types/brand'

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
  }),
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    Link: ({ to, children }: { to?: string; children?: ReactNode }) => (
      <a href={to}>{children}</a>
    ),
  }
})

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 44,
    measureElement: () => undefined,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 44,
      })),
  }),
}))

function row(over: Partial<PipelineRunRow>): PipelineRunRow {
  return {
    id: 'r1',
    app: 'web',
    pipeline: 'ci',
    number: 1,
    status: 'succeeded',
    trigger: 'push',
    created_at: new Date().toISOString(),
    approval_pending: false,
    hold_pending: false,
    can_decide: false,
    ...over,
  }
}

const SUMMARY: PipelineSummary = {
  running: 2,
  succeeded_24h: 3,
  failed_24h: 1,
  cancelled_24h: 0,
  waiting_approval: 1,
  held: 0,
  success_rate_24h: 0.75,
}

interface Fixture {
  runs: PipelineRunRow[]
  requests: string[]
  posts: string[]
}

function stubApi(fx: Fixture) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.href
            : input.url
      fx.requests.push(url)
      const json = (body: unknown) =>
        Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))
      if (init?.method === 'POST') {
        fx.posts.push(url)
        return json({})
      }
      if (url.startsWith('/api/v1/pipelines/summary')) {
        return json(SUMMARY)
      }
      if (url.startsWith('/api/v1/apps')) {
        return json([{ name: 'web' }, { name: 'api' }])
      }
      const params = new URL(url, 'http://x').searchParams
      const status = params.get('status')
      const runs = fx.runs.filter((r) => {
        if (status === 'waiting_approval') return r.approval_pending
        if (status === 'held') return r.hold_pending
        if (status) return r.status === status
        return true
      })
      const page: PipelineRunRowsPage = { runs }
      return json(page)
    }),
  )
}

function renderPage() {
  const Page = Route.options.component!
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <Page />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Pipelines page', () => {
  it('shows the empty state with a YAML example and an app picker', async () => {
    stubApi({ runs: [], requests: [], posts: [] })
    renderPage()
    expect(await screen.findByText('No pipeline runs yet')).toBeTruthy()
    expect(screen.getByLabelText('Example pipeline').textContent).toContain(
      'version: 1',
    )
    await userEvent.click(
      screen.getByRole('button', { name: /create a pipeline/i }),
    )
    expect(await screen.findByText('Choose an app')).toBeTruthy()
    expect(await screen.findByRole('button', { name: 'web' })).toBeTruthy()
  })

  it('renders summary tiles and runs when populated', async () => {
    stubApi({
      runs: [
        row({ id: 'a', number: 7, status: 'running' }),
        row({ id: 'b', number: 8, app: 'api', pipeline: 'release' }),
      ],
      requests: [],
      posts: [],
    })
    renderPage()
    expect(await screen.findByText('api')).toBeTruthy()
    expect(screen.getByText('75%')).toBeTruthy()
    expect(screen.getByText('Failed (24h)')).toBeTruthy()
    expect(screen.getByText(/ci #7/)).toBeTruthy()
    expect(screen.queryByText('Needs attention')).toBeNull()
  })

  it('shows the attention strip and posts an approval inline', async () => {
    const fx: Fixture = {
      runs: [
        row({
          id: 'gate',
          number: 4,
          status: 'waiting_approval',
          approval_pending: true,
          approval_id: 9,
          can_decide: true,
        }),
        row({
          id: 'bad',
          number: 5,
          status: 'failed',
          reason: 'job test failed',
        }),
      ],
      requests: [],
      posts: [],
    }
    stubApi(fx)
    renderPage()
    expect(await screen.findByText('Needs attention')).toBeTruthy()
    expect(screen.getByText('job test failed')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: /approve/i }))
    await waitFor(() =>
      expect(fx.posts).toContain(
        '/api/v1/apps/web/pipeline-runs/gate/approvals/9',
      ),
    )
  })

  it('hides approve and reject when the caller cannot decide', async () => {
    stubApi({
      runs: [
        row({
          id: 'held',
          status: 'queued',
          hold_pending: true,
          can_decide: false,
        }),
      ],
      requests: [],
      posts: [],
    })
    renderPage()
    expect(await screen.findByText('Needs attention')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /approve/i })).toBeNull()
  })

  it('sends the pipeline filter to the API', async () => {
    const fx: Fixture = {
      runs: [row({ id: 'a' }), row({ id: 'b', status: 'failed', number: 2 })],
      requests: [],
      posts: [],
    }
    stubApi(fx)
    renderPage()
    await screen.findByText(/ci #1/)
    await userEvent.type(screen.getByLabelText('Pipeline name'), 'ci')
    await waitFor(() =>
      expect(fx.requests.some((u) => u.includes('pipeline=ci'))).toBe(true),
    )
  })
})
