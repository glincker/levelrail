import { describe, expect, it, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import SecretProvidersSection from './SecretProvidersSection'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('SecretProvidersSection', () => {
  it('lists providers and never shows a token value', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              providers: [
                {
                  name: 'prod',
                  type: 'doppler',
                  project: 'api',
                  environment: 'prd',
                  self_hosted: false,
                  has_token: true,
                  updated_at: '2026-11-07T00:00:00Z',
                },
              ],
            }),
          ),
        ),
      ),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SecretProvidersSection />
      </QueryClientProvider>,
    )
    expect(await screen.findByText('prod')).toBeTruthy()
    expect(screen.getByText('api / prd', { exact: false })).toBeTruthy()
  })
})
