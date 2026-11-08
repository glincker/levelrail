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

  it('blocks Save for 1Password when the vault is empty', async () => {
    const methods: (string | undefined)[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        methods.push(init?.method)
        return Promise.resolve(
          new Response(
            JSON.stringify({
              providers: [
                {
                  name: 'vaults',
                  type: 'onepassword',
                  project: '',
                  environment: '',
                  base_url: 'https://connect.internal:8080',
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
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    expect((await screen.findByRole('alert')).textContent).toContain(
      'secretProviders.errVault',
    )
    expect(methods).not.toContain('PUT')
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

  it('Add provider Save sends one PUT with the typed token and never echoes it', async () => {
    window.HTMLElement.prototype.hasPointerCapture = () => false
    window.HTMLElement.prototype.scrollIntoView = () => undefined
    const calls: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, init })
        if (init?.method === 'PUT') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                name: 'op1',
                type: 'onepassword',
                project: 'Prod',
                self_hosted: true,
                has_token: true,
                updated_at: '2026-11-08T00:00:00Z',
              }),
            ),
          )
        }
        return Promise.resolve(new Response(JSON.stringify({ providers: [] })))
      }),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SecretProvidersSection />
      </QueryClientProvider>,
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'secretProviders.add' }),
    )
    await userEvent.type(screen.getByLabelText('secretProviders.name'), 'op1')
    await userEvent.click(screen.getByLabelText('secretProviders.type'))
    await userEvent.click(
      await screen.findByRole('option', { name: '1Password' }),
    )
    await userEvent.type(
      screen.getByLabelText('secretProviders.projectOnePassword'),
      'Prod',
    )
    await userEvent.type(
      screen.getByLabelText('secretProviders.baseUrlConnect'),
      'https://connect.internal:8080',
    )
    await userEvent.type(
      screen.getByLabelText('secretProviders.token'),
      'tok-123',
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    await waitFor(() => {
      expect(calls.filter((c) => c.init?.method === 'PUT')).toHaveLength(1)
    })
    const put = calls.find((c) => c.init?.method === 'PUT')
    expect(put?.url).toContain('/op1')
    expect(JSON.parse(put?.init?.body as string)).toMatchObject({
      type: 'onepassword',
      project: 'Prod',
      token: 'tok-123',
    })
    await waitFor(() => {
      expect(screen.queryByRole('form')).toBeNull()
    })
    expect(document.body.textContent).not.toContain('tok-123')
  })

  it('maps the Infisical client ID and secret to the prefixed credential', async () => {
    window.HTMLElement.prototype.hasPointerCapture = () => false
    window.HTMLElement.prototype.scrollIntoView = () => undefined
    const calls: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, init })
        if (init?.method === 'PUT') return Promise.resolve(new Response('{}'))
        return Promise.resolve(new Response(JSON.stringify({ providers: [] })))
      }),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SecretProvidersSection />
      </QueryClientProvider>,
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'secretProviders.add' }),
    )
    await userEvent.type(screen.getByLabelText('secretProviders.name'), 'inf')
    await userEvent.click(screen.getByLabelText('secretProviders.type'))
    await userEvent.click(
      await screen.findByRole('option', { name: '1Password' }),
    )
    fireEvent.change(screen.getByLabelText('secretProviders.baseUrlConnect'), {
      target: { value: 'https://connect.internal:8080' },
    })
    await userEvent.type(screen.getByLabelText('secretProviders.token'), 'tok')
    await userEvent.click(screen.getByLabelText('secretProviders.type'))
    await userEvent.click(
      await screen.findByRole('option', { name: 'Infisical' }),
    )
    await userEvent.type(
      screen.getByLabelText('secretProviders.projectInfisical'),
      'ws1',
    )
    await userEvent.type(
      screen.getByLabelText('secretProviders.envInfisical'),
      'prod',
    )
    await userEvent.clear(
      screen.getByLabelText('secretProviders.tokenOrIdentity'),
    )
    await userEvent.type(
      screen.getByLabelText('secretProviders.clientId'),
      'cid',
    )
    await userEvent.type(
      screen.getByLabelText('secretProviders.clientSecret'),
      'shh',
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true)
    })
    const put = calls.find((c) => c.init?.method === 'PUT')
    expect(JSON.parse(put?.init?.body as string)).toMatchObject({
      type: 'infisical',
      token: 'infisical-identity:cid:shh',
    })
    await waitFor(() => {
      expect(screen.queryByRole('form')).toBeNull()
    })
    expect(document.body.textContent).not.toContain('shh')
  })

  function stubEditable(row: Record<string, unknown>) {
    const calls: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, init })
        if (init?.method === 'PUT') return Promise.resolve(new Response('{}'))
        return Promise.resolve(
          new Response(JSON.stringify({ providers: [row] })),
        )
      }),
    )
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SecretProvidersSection />
      </QueryClientProvider>,
    )
    return calls
  }

  const infisicalRow = {
    name: 'inf',
    type: 'infisical',
    project: 'ws1',
    environment: 'prod',
    client_id: 'cid-stored',
    self_hosted: false,
    has_token: true,
    updated_at: '2026-11-08T00:00:00Z',
  }

  it('prefills the Infisical client ID on edit and keeps the secret empty', async () => {
    const calls = stubEditable(infisicalRow)
    await userEvent.click(
      await screen.findByRole('button', { name: 'secretProviders.edit' }),
    )
    expect(
      screen.getByLabelText<HTMLInputElement>('secretProviders.clientId').value,
    ).toBe('cid-stored')
    expect(
      screen.getByLabelText<HTMLInputElement>('secretProviders.clientSecret')
        .value,
    ).toBe('')
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true)
    })
    const body = JSON.parse(
      calls.find((c) => c.init?.method === 'PUT')?.init?.body as string,
    ) as Record<string, unknown>
    expect(body.token).toBeUndefined()
  })

  it('requires the client secret only once the client ID changes', async () => {
    const calls = stubEditable(infisicalRow)
    await userEvent.click(
      await screen.findByRole('button', { name: 'secretProviders.edit' }),
    )
    await userEvent.type(screen.getByLabelText('secretProviders.clientId'), 'x')
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    expect((await screen.findByRole('alert')).textContent).toContain(
      'secretProviders.errIdentityReenter',
    )
    expect(calls.some((c) => c.init?.method === 'PUT')).toBe(false)
    await userEvent.type(
      screen.getByLabelText('secretProviders.clientSecret'),
      'shh',
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    await waitFor(() => {
      expect(calls.some((c) => c.init?.method === 'PUT')).toBe(true)
    })
    const body = JSON.parse(
      calls.find((c) => c.init?.method === 'PUT')?.init?.body as string,
    ) as Record<string, unknown>
    expect(body.token).toBe('infisical-identity:cid-storedx:shh')
  })

  it('blocks Save client-side when the 1Password vault is empty', async () => {
    const calls = stubEditable({
      name: 'vaults',
      type: 'onepassword',
      project: 'Prod',
      base_url: 'https://connect.internal:8080',
      self_hosted: true,
      has_token: true,
      updated_at: '2026-11-08T00:00:00Z',
    })
    await userEvent.click(
      await screen.findByRole('button', { name: 'secretProviders.edit' }),
    )
    await userEvent.clear(
      screen.getByLabelText('secretProviders.projectOnePassword'),
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'secretProviders.save' }),
    )
    expect((await screen.findByRole('alert')).textContent).toContain(
      'secretProviders.errVault',
    )
    expect(calls.some((c) => c.init?.method === 'PUT')).toBe(false)
  })
})
