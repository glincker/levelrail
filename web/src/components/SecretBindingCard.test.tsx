import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from '@/components/ui/toast'
import { SecretBindingCard } from './SecretBindingCard'
import type { Brand } from '../types/brand'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

vi.mock('../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: 'https://test.example/docs',
    DiscussionsURL: '',
  }),
}))

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
      <SecretBindingCard />
    </QueryClientProvider>,
  )
}

describe('SecretBindingCard', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.mocked(toast.add).mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows all bound with no action when nothing is legacy', async () => {
    fetchMock.mockResolvedValue(json({ total: 4, bound: 4, legacy: 0 }))
    renderCard()
    expect(await screen.findByText('All bound')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /bind now/i })).toBeNull()
  })

  it('renders nothing when secrets are not configured', async () => {
    fetchMock.mockResolvedValue(json({ error: 'no master key' }, 501))
    const { container } = renderCard()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(container.textContent).toBe('')
  })

  it('rebinds legacy values with POST and lists failures', async () => {
    let legacy = 3
    fetchMock.mockImplementation((_url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        legacy = 0
        return Promise.resolve(
          json({
            scanned: 4,
            rebound: 2,
            alreadyBound: 1,
            changed: 0,
            failedCount: 1,
            failed: [
              {
                owner: 'web',
                key: 'API_KEY',
                reason: 'ciphertext is bound to a different slot',
              },
            ],
            remaining: 0,
          }),
        )
      }
      return Promise.resolve(json({ total: 4, bound: 4 - legacy, legacy }))
    })
    renderCard()
    expect(await screen.findByText('3 legacy')).toBeTruthy()

    await userEvent.click(screen.getByRole('button', { name: /bind now/i }))

    await waitFor(() => {
      expect(toast.add).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'error' }),
      )
    })
    expect(
      fetchMock.mock.calls.some(
        ([url, init]) =>
          url === '/api/v1/system/secrets/rebind' &&
          (init as RequestInit | undefined)?.method === 'POST',
      ),
    ).toBe(true)
    expect(
      await screen.findByText(
        'web/API_KEY: ciphertext is bound to a different slot',
      ),
    ).toBeTruthy()
    expect(await screen.findByText('All bound')).toBeTruthy()
  })
})
