import { describe, expect, it, vi, afterEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import SecretProvidersSection from './SecretProvidersSection'

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

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

  it('requires the Connect URL for 1Password and sends no environment', async () => {
    const calls: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, init })
        if (init?.method === 'PUT') return Promise.resolve(new Response('{}'))
        return Promise.resolve(
          new Response(
            JSON.stringify({
              providers: [
                {
                  name: 'vaults',
                  type: 'onepassword',
                  project: 'Prod',
                  environment: '',
                  self_hosted: true,
                  has_token: true,
                  updated_at: '2026-11-07T00:00:00Z',
                },
              ],
            }),
          ),
        )
      }),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SecretProvidersSection />
      </QueryClientProvider>,
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'secretProviders.edit' }),
    )
    expect(screen.queryByLabelText('secretProviders.envDoppler')).toBeNull()
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    expect((await screen.findByRole('alert')).textContent).toContain(
      'secretProviders.errBaseUrl',
    )
    fireEvent.change(screen.getByLabelText('secretProviders.baseUrlConnect'), {
      target: { value: 'https://connect.internal:8080' },
    })
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true)
    })
    const put = calls.find((c) => c.init?.method === 'PUT')
    const body = JSON.parse(put?.init?.body as string) as Record<
      string,
      unknown
    >
    expect(body).toMatchObject({
      type: 'onepassword',
      project: 'Prod',
      base_url: 'https://connect.internal:8080',
    })
    expect(body).not.toHaveProperty('environment')
  })

  it('shows the Connect server origin and vault for 1Password rows', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              providers: [
                {
                  name: 'op1',
                  type: 'onepassword',
                  project: 'Infra',
                  base_url: 'https://user:pw@connect.internal:8080/x?k=v',
                  self_hosted: true,
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
    expect(
      await screen.findByText('https://connect.internal:8080 / Infra', {
        exact: false,
      }),
    ).toBeTruthy()
    expect(screen.queryByText('pw', { exact: false })).toBeNull()
  })
})
