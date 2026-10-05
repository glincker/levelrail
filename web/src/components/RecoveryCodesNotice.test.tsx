import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RecoveryCodesNotice } from './RecoveryCodesNotice'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => (
    <a href="/settings/security">{children}</a>
  ),
  useRouterState: ({
    select,
  }: {
    select: (s: { location: { pathname: string } }) => unknown
  }) => select({ location: { pathname: '/apps' } }),
}))

function stubStatus(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      } as unknown as Response),
    ),
  )
}

function renderNotice(inline?: boolean) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RecoveryCodesNotice inline={inline} />
    </QueryClientProvider>,
  )
}

describe('RecoveryCodesNotice', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    window.sessionStorage.clear()
  })

  it('shows the prompt and a way to regenerate when the server asks for it', async () => {
    stubStatus({
      enabled: true,
      recovery_codes_remaining: 0,
      recovery_codes_need_regeneration: true,
    })
    renderNotice()
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.getAllByRole('link').length).toBe(1)
  })

  it('stays out of the way on the settings page', async () => {
    stubStatus({
      enabled: true,
      recovery_codes_remaining: 0,
      recovery_codes_need_regeneration: true,
    })
    renderNotice(true)
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(screen.queryAllByRole('link').length).toBe(0)
  })

  it('renders nothing when no regeneration is needed', async () => {
    const fetchSpy = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({ enabled: true, recovery_codes_remaining: 8 }),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchSpy)
    renderNotice()
    await waitFor(() => expect(fetchSpy).toHaveBeenCalled())
    expect(screen.queryByRole('alert')).toBeNull()
  })
})
