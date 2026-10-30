import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClientProvider } from '@tanstack/react-query'
import { ConsentCard } from './ConsentCard'
import { createTestQueryClient } from '@/test/testQueryClient'

function stub(scopes: string[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('dev-mode')) {
        return Promise.resolve(
          Response.json({ enabled: false, experimental: ['mcp-http'] }),
        )
      }
      return Promise.resolve(
        Response.json({
          id: 'r1',
          client_id: 'c1',
          client_name: 'Agent',
          redirect_host: 'client.example',
          requested_scopes: scopes,
          grantable_scopes: scopes,
          resource: 'x',
          access_ttl_seconds: 3600,
          refresh_ttl_seconds: 86400,
          expires_at: '2026-10-30T00:00:00Z',
        }),
      )
    }),
  )
}

function renderCard() {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <ConsentCard requestId="r1" />
    </QueryClientProvider>,
  )
}

describe('ConsentCard scopes', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists each granted scope and words the sensitive approve distinctly', async () => {
    stub(['mcp:read', 'mcp:read:sensitive'])
    renderCard()
    expect(
      await screen.findByRole('button', { name: 'approveSensitive' }),
    ).toBeInTheDocument()
    expect(screen.getAllByTestId('consent-scope')).toHaveLength(2)
  })

  it('uses the plain approve label for a read-only grant', async () => {
    stub(['mcp:read'])
    renderCard()
    expect(
      await screen.findByRole('button', { name: 'approve' }),
    ).toBeInTheDocument()
  })
})
