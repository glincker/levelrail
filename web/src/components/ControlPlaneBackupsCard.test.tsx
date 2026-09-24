import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ControlPlaneBackupsCard } from './ControlPlaneBackupsCard'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const backup = {
  name: 'levelrail-20260923T030000Z.db',
  size_bytes: 2048,
  created_at: '2026-09-23T03:00:00Z',
  sha256: 'abcdef0123456789abcdef',
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
      <ControlPlaneBackupsCard />
    </QueryClientProvider>,
  )
}

describe('ControlPlaneBackupsCard', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists backups with a download link and the master key notice', async () => {
    fetchMock.mockResolvedValue(json([backup]))
    renderCard()
    expect(await screen.findByText(backup.name)).toBeTruthy()
    expect(screen.getByText('abcdef012345')).toBeTruthy()
    expect(screen.getByText(/master key is not included/i)).toBeTruthy()
    const link = screen.getByText('Download').closest('a') as HTMLAnchorElement
    expect(link.getAttribute('href')).toBe(
      `/api/v1/system/backups/${backup.name}/download`,
    )
  })

  it('shows the empty state', async () => {
    fetchMock.mockResolvedValue(json([]))
    renderCard()
    expect(await screen.findByText(/no backups yet/i)).toBeTruthy()
  })

  it('hides the card when the list is forbidden', async () => {
    fetchMock.mockResolvedValue(json({ error: 'forbidden' }, 403))
    const { container } = renderCard()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    await waitFor(() => {
      expect(container.textContent).toBe('')
    })
  })

  it('creates a backup with POST and refreshes the list', async () => {
    fetchMock.mockImplementation((_url: string, init?: RequestInit) =>
      Promise.resolve(init?.method === 'POST' ? json(backup, 201) : json([])),
    )
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: /back up now/i }),
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith('/api/v1/system/backups', {
        method: 'POST',
      })
    })
    await waitFor(() => {
      const gets = fetchMock.mock.calls.filter(
        (c) => (c[1] as RequestInit | undefined)?.method === undefined,
      )
      expect(gets.length).toBeGreaterThanOrEqual(2)
    })
  })

  it('deletes a backup after confirming', async () => {
    fetchMock.mockImplementation((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'DELETE'
          ? new Response(null, { status: 204 })
          : json([backup]),
      ),
    )
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: /^delete$/i }),
    )
    const buttons = await screen.findAllByRole('button', { name: /^delete$/i })
    await userEvent.click(buttons.at(-1) as HTMLElement)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        `/api/v1/system/backups/${backup.name}`,
        { method: 'DELETE' },
      )
    })
  })
})
