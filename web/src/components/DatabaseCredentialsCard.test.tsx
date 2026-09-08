import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabaseCredentialsCard } from './DatabaseCredentialsCard'

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <DatabaseCredentialsCard name="main" />
    </QueryClientProvider>,
  )
}

describe('DatabaseCredentialsCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('does not fetch credentials until Reveal is clicked', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    renderCard()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('fetches and shows credentials with the password masked by default', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => ({
        host: 'db-main',
        port: 5432,
        database: 'main',
        username: 'main',
        password: 's3cr3t',
        url: 'postgres://main:s3cr3t@db-main:5432/main',
      }),
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Reveal credentials' }))

    await waitFor(() => {
      expect(screen.getByText('db-main')).toBeInTheDocument()
    })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/databases/main/credentials',
    )
    expect(screen.queryByText('s3cr3t')).not.toBeInTheDocument()
  })

  it('reveals the password when the show/hide toggle is clicked', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => ({
        host: 'db-main',
        port: 5432,
        database: 'main',
        username: 'main',
        password: 's3cr3t',
        url: 'postgres://main:s3cr3t@db-main:5432/main',
      }),
    })
    vi.stubGlobal('fetch', fetchMock)

    const user = userEvent.setup()
    renderCard()

    await user.click(screen.getByRole('button', { name: 'Reveal credentials' }))
    await waitFor(() => {
      expect(screen.getByText('db-main')).toBeInTheDocument()
    })

    await user.click(screen.getByRole('button', { name: 'Show password' }))

    expect(screen.getByText('s3cr3t')).toBeInTheDocument()
  })
})
