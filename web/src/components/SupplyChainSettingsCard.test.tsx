import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SupplyChainSettingsCard } from './SupplyChainSettingsCard'
import type { SupplyChainSettings } from '../types/supplyChain'

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <SupplyChainSettingsCard appName="web" />
    </QueryClientProvider>,
  )
}

describe('SupplyChainSettingsCard', () => {
  let cfg: SupplyChainSettings
  let puts: unknown[]
  let overrides: unknown[]

  beforeEach(() => {
    puts = []
    overrides = []
    cfg = {
      app: 'web',
      scan_enabled: false,
      scan_gate: 'off',
      server_enabled: true,
      build_attest: true,
      scanner: 'trivy',
      scanner_image: 'docker.io/aquasec/trivy:0.65.0',
      override_armed: false,
    }
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        const method = init?.method ?? 'GET'
        if (url === '/api/v1/apps/web/supply-chain' && method === 'PUT') {
          const body = JSON.parse(
            init?.body as string,
          ) as Partial<SupplyChainSettings>
          puts.push(body)
          cfg = { ...cfg, ...body }
          return Promise.resolve(jsonResponse(cfg))
        }
        if (url === '/api/v1/apps/web/supply-chain/override') {
          overrides.push(JSON.parse(init?.body as string))
          cfg = { ...cfg, override_armed: true, override_reason: 'outage' }
          return Promise.resolve(jsonResponse(cfg))
        }
        if (url === '/api/v1/apps/web/supply-chain') {
          return Promise.resolve(jsonResponse(cfg))
        }
        return Promise.resolve(jsonResponse({ error: 'nope' }, 404))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('is off by default and turns scanning on', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Turn on' }))
    await waitFor(() => {
      expect(puts).toEqual([{ scan_enabled: true }])
    })
    expect(
      await screen.findByRole('radiogroup', { name: 'Scan gate' }),
    ).toBeTruthy()
  })

  it('turning scanning off also resets the gate', async () => {
    cfg = { ...cfg, scan_enabled: true, scan_gate: 'warn' }
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Turn off' }))
    await waitFor(() => {
      expect(puts).toEqual([{ scan_enabled: false, scan_gate: 'off' }])
    })
  })

  it('changes the gate', async () => {
    cfg = { ...cfg, scan_enabled: true }
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('radio', { name: /Block on critical/ }),
    )
    await waitFor(() => {
      expect(puts).toEqual([{ scan_gate: 'block_on_critical' }])
    })
  })

  it('arms a one-shot override only with a reason', async () => {
    cfg = { ...cfg, scan_enabled: true, scan_gate: 'block_on_critical' }
    const user = userEvent.setup()
    renderCard()
    const arm = await screen.findByRole('button', { name: 'Arm override' })
    expect(arm.hasAttribute('disabled')).toBe(true)
    await user.type(
      screen.getByPlaceholderText('Why this release may go live'),
      'outage',
    )
    await user.click(arm)
    await waitFor(() => {
      expect(overrides).toEqual([{ reason: 'outage' }])
    })
    expect(await screen.findByText('Armed: outage')).toBeTruthy()
  })

  it('says when builds record no SBOM or the server refuses scans', async () => {
    cfg = { ...cfg, build_attest: false, server_enabled: false }
    renderCard()
    expect(await screen.findByText(/APP_BUILD_ATTEST=true/)).toBeTruthy()
    expect(screen.getByText(/APP_SCAN_ENABLED=true/)).toBeTruthy()
    const on = screen.getByRole('button', { name: 'Turn on' })
    expect(on.hasAttribute('disabled')).toBe(true)
  })
})
