import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { parseRecipients, recipientProblem } from '../lib/controlPlaneDr'
import type { ControlPlaneDr } from '../queries/controlPlaneDr'
import { ControlPlaneDrCard } from './ControlPlaneDrCard'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const base: ControlPlaneDr = {
  enabled: true,
  configured: true,
  target_id: 'bkt_1',
  install_id: 'inst-1',
  recipients: ['age1abc'],
  schedule: '0 2 * * *',
  drill_schedule: '0 5 * * 0',
  retain_daily: 7,
  retain_weekly: 4,
  retain_monthly: 6,
  escrow_target_id: '',
  last_drill: {
    ok: true,
    partial: true,
    detail: 'partial',
    duration_ms: 1200,
    at: '2026-09-25T05:00:00Z',
  },
  drill_identity_configured: false,
  backup_running: false,
  drill_running: false,
  checklist: {
    destination_chosen: true,
    recipient_set: true,
    escrow_acknowledged: false,
    drill_passed: true,
  },
  warnings: [
    { code: 'escrow_missing', message: 'No escrow bundle has been generated.' },
  ],
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function renderCard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ControlPlaneDrCard />
    </QueryClientProvider>,
  )
}

describe('ControlPlaneDrCard', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders nothing when the endpoint is unavailable', async () => {
    fetchMock.mockResolvedValue(json({ error: 'not configured' }, 501))
    const { container } = renderCard()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('shows warnings, checklist progress and a partial drill note', async () => {
    fetchMock.mockImplementation((url: string) =>
      Promise.resolve(
        url.includes('/storage/destinations') ? json([]) : json(base),
      ),
    )
    renderCard()
    expect(await screen.findByText('Disaster recovery')).toBeInTheDocument()
    expect(
      screen.getByText('No escrow bundle has been generated.'),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Set up disaster recovery \(3 of 4\)/),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/checksum only, decryption untested/),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Run now/ })).toBeEnabled()
  })

  it('starts a backup and a drill', async () => {
    const user = userEvent.setup()
    fetchMock.mockImplementation((url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        return Promise.resolve(json({ started: true }, 202))
      }
      return Promise.resolve(
        url.includes('/storage/destinations') ? json([]) : json(base),
      )
    })
    renderCard()
    await user.click(await screen.findByRole('button', { name: /Run now/ }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/system/control-plane-dr/run',
        expect.objectContaining({ method: 'POST' }),
      )
    })
    await user.click(screen.getByRole('button', { name: 'Run drill' }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/system/control-plane-dr/drill',
        expect.objectContaining({ method: 'POST' }),
      )
    })
  })

  it('disables the run buttons until a destination and recipient exist', async () => {
    fetchMock.mockImplementation((url: string) =>
      Promise.resolve(
        url.includes('/storage/destinations')
          ? json([])
          : json({ ...base, configured: false }),
      ),
    )
    renderCard()
    expect(
      await screen.findByRole('button', { name: /Run now/ }),
    ).toBeDisabled()
  })
})

describe('recipient helpers', () => {
  it('splits on whitespace and commas', () => {
    expect(parseRecipients('age1a\nage1b, age1c  ')).toEqual([
      'age1a',
      'age1b',
      'age1c',
    ])
  })

  it('rejects private keys and non-age text', () => {
    expect(recipientProblem(['AGE-SECRET-KEY-1ABC'])).toMatch(/private key/)
    expect(recipientProblem(['ssh-rsa AAAA'])).toMatch(/not an age public key/)
    expect(recipientProblem(['age1abc'])).toBeNull()
    expect(recipientProblem([])).toBeNull()
  })
})
