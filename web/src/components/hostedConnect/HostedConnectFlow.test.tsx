import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { DatabasesI18n } from '../../test/databasesI18n'
import { HostedConnectFlow } from './HostedConnectFlow'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

const URL_VALUE =
  'postgres://user:secret@ep-example-123.us-east-2.aws.neon.tech/db?sslmode=require'

const providers = [
  {
    id: 'neon',
    label: 'Neon',
    brand: 'neon',
    engines: ['postgres'],
    tls_mode: 'verify-full',
    host_hint: 'ep-name-123456.region.aws.neon.tech',
    docs_url: 'https://example.test/docs',
  },
]

const parsed = {
  engine: 'postgres',
  host: 'ep-example-123.us-east-2.aws.neon.tech',
  port: 5432,
  username: 'user',
  database: 'db',
  tls_mode: 'require',
  provider: 'neon',
  has_password: true,
  suggested_name: 'neon-ep-example-123',
}

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function mockApi(sent: string[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((path: string, init?: RequestInit) => {
      sent.push(`${init?.method ?? 'GET'} ${path}`)
      if (path.endsWith('/providers')) return json(providers)
      if (path.endsWith('/parse')) return json(parsed)
      if (path.endsWith('/test')) {
        return json({
          status: 'unreachable',
          reason: 'connection refused',
          latency_ms: 0,
        })
      }
      return json({ error: 'no' }, 500)
    }),
  )
}

function show() {
  render(
    <DatabasesI18n>
      <QueryClientProvider client={new QueryClient()}>
        <Dialog open>
          <DialogContent>
            <HostedConnectFlow onClose={() => undefined} />
          </DialogContent>
        </Dialog>
      </QueryClientProvider>
    </DatabasesI18n>,
  )
}

describe('HostedConnectFlow', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('gallery to pasted URL to parsed fields to failed probe offers save anyway', async () => {
    const sent: string[] = []
    mockApi(sent)
    show()

    fireEvent.click(await screen.findByRole('button', { name: /Neon/ }))

    const paste = screen.getByLabelText('Connection string')
    fireEvent.change(paste, { target: { value: URL_VALUE } })
    fireEvent.blur(paste)

    expect(
      await screen.findByDisplayValue('ep-example-123.us-east-2.aws.neon.tech'),
    ).toBeTruthy()
    expect(screen.getByDisplayValue('neon-ep-example-123')).toBeTruthy()
    expect(sent).toContain('POST /api/v1/external-databases/parse')

    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(screen.queryByText(/Save even though/)).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
    expect(await screen.findByText(/Save even though/)).toBeTruthy()
    expect(screen.getByText('connection refused')).toBeTruthy()
  })
})
