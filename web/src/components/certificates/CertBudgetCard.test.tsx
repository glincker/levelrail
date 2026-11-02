import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { CertBudgetCard } from './CertBudgetCard'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/x">{children}</a>
  ),
}))

function budget(issued: number, warn = 40) {
  return {
    issued,
    warn,
    window_days: 7,
    over: issued > warn,
    checked_at: '2026-11-02T00:00:00Z',
  }
}

function stub(res: () => Response, calls: string[] = []) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push(`${init?.method ?? 'GET'} ${url}`)
      if (init?.method === 'PUT')
        return Promise.resolve(new Response('{}', { status: 200 }))
      return Promise.resolve(res())
    }),
  )
}

function renderCard() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <AllI18n>
        <CertBudgetCard />
      </AllI18n>
    </QueryClientProvider>,
  )
}

const ok = (b: object) => () => new Response(JSON.stringify(b), { status: 200 })

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('CertBudgetCard', () => {
  it('shows ok', async () => {
    stub(ok(budget(5)))
    renderCard()
    expect(await screen.findByText('Within budget')).toBeTruthy()
  })

  it('shows near', async () => {
    stub(ok(budget(35)))
    renderCard()
    expect(await screen.findByText('Close to the limit')).toBeTruthy()
  })

  it('shows over with what it means', async () => {
    stub(ok(budget(50)))
    renderCard()
    expect(await screen.findByText('Over the limit')).toBeTruthy()
    expect(screen.getByText(/break first deploys/)).toBeTruthy()
  })

  it('shows loading first', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise<Response>(() => undefined)),
    )
    renderCard()
    expect(screen.getByRole('status')).toBeTruthy()
  })

  it('shows an error with a retry', async () => {
    stub(() => new Response('{"error":"boom"}', { status: 500 }))
    renderCard()
    expect(
      await screen.findByText('Could not load the certificate budget.'),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeTruthy()
  })

  it('saves the threshold as a runtime setting', async () => {
    const calls: string[] = []
    stub(ok(budget(5)), calls)
    renderCard()
    const input = await screen.findByLabelText('Warn at')
    fireEvent.change(input, { target: { value: '60' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(calls).toContain(
        'PUT /api/v1/settings/runtime/certificates.weekly_issue_warn',
      ),
    )
  })
})
