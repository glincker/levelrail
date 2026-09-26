import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SupplyChainSection, VulnCountPills } from './SupplyChainSection'
import type {
  SbomSummary,
  SupplyChainSettings,
  VulnReport,
} from '../types/supplyChain'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))

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

const SBOM: SbomSummary = {
  deployment_id: 'da_1',
  format: 'spdx',
  package_count: 214,
  types: [{ type: 'apk', count: 200 }],
  licenses: [
    { license: 'MIT', count: 120 },
    { license: 'GPL-2.0-only', count: 30 },
  ],
  unlicensed: 7,
  top_packages: [
    { name: 'busybox', version: '1.36.1', type: 'apk' },
    { name: 'musl', version: '1.2.4', type: 'apk' },
  ],
  provenance: true,
  available: true,
  bytes: 1024,
  generated_at: '2026-09-26T12:00:00Z',
  download_url: '/api/v1/apps/web/deployments/da_1/sbom?download=true',
}

const SCANNED: VulnReport = {
  deployment_id: 'da_1',
  scan: {
    status: 'ok',
    scanner: 'trivy',
    counts: { critical: 2, high: 1, medium: 0, low: 4, unknown: 0 },
    fixable: 3,
    top: [
      {
        id: 'CVE-2024-0001',
        package: 'openssl',
        version: '3.1.4',
        fixed_version: '3.1.5',
        severity: 'critical',
      },
    ],
  },
  gate: { action: 'block', reason: '2 critical vulnerabilities' },
}

function settings(
  over: Partial<SupplyChainSettings> = {},
): SupplyChainSettings {
  return {
    app: 'web',
    scan_enabled: true,
    scan_gate: 'block_on_critical',
    server_enabled: true,
    build_attest: true,
    scanner: 'trivy',
    scanner_image: 'docker.io/aquasec/trivy:0.65.0',
    override_armed: false,
    ...over,
  }
}

function renderSection(sbomPackages: number | null | undefined) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <SupplyChainSection
        appName="web"
        deploymentId="da_1"
        sbomPackages={sbomPackages}
      />
    </QueryClientProvider>,
  )
}

describe('SupplyChainSection', () => {
  let cfg: SupplyChainSettings
  let vulns: VulnReport
  let scans: number
  let fetched: string[]

  beforeEach(() => {
    cfg = settings()
    vulns = SCANNED
    scans = 0
    fetched = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = urlOf(input)
        fetched.push(url)
        const method = init?.method ?? 'GET'
        if (url.endsWith('/sbom')) return Promise.resolve(jsonResponse(SBOM))
        if (url.endsWith('/vulnerabilities'))
          return Promise.resolve(jsonResponse(vulns))
        if (url.endsWith('/scan') && method === 'POST') {
          scans += 1
          return Promise.resolve(jsonResponse(SCANNED))
        }
        if (url.endsWith('/supply-chain'))
          return Promise.resolve(jsonResponse(cfg))
        return Promise.resolve(jsonResponse({ error: 'nope' }, 404))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('explains itself and fetches nothing when the deploy has no SBOM', () => {
    renderSection(null)
    expect(
      screen.getByText(/No software bill of materials was recorded/),
    ).toBeTruthy()
    expect(fetched.filter((u) => u.includes('/sbom'))).toHaveLength(0)
  })

  it('shows packages, licenses, vulnerability pills and the gate result', async () => {
    renderSection(214)
    expect(await screen.findByText('214')).toBeTruthy()
    expect(
      screen.getByText(/MIT 120, GPL-2.0-only 30, 7 without a license/),
    ).toBeTruthy()
    expect(screen.getByText('busybox 1.36.1')).toBeTruthy()
    expect(await screen.findByText('2 critical')).toBeTruthy()
    expect(screen.getByText('1 high')).toBeTruthy()
    expect(screen.getByText('4 low')).toBeTruthy()
    expect(screen.queryByText('0 medium')).toBeNull()
    expect(screen.getByText(/fixed in 3.1.5/)).toBeTruthy()
    expect(
      screen.getByText('Blocked, the previous release kept serving'),
    ).toBeTruthy()
  })

  it('scans on demand', async () => {
    const user = userEvent.setup()
    renderSection(214)
    await user.click(await screen.findByRole('button', { name: 'Scan now' }))
    await waitFor(() => {
      expect(scans).toBe(1)
    })
  })

  it('disables Scan now and says why when scanning is off for the app', async () => {
    cfg = settings({ scan_enabled: false })
    renderSection(214)
    const button = await screen.findByRole('button', { name: 'Scan now' })
    await waitFor(() => {
      expect(button.hasAttribute('disabled')).toBe(true)
    })
    expect(
      await screen.findByText(/Turn on scanning in Deploy settings/),
    ).toBeTruthy()
  })

  it('reports a failed scan instead of counts', async () => {
    vulns = {
      deployment_id: 'da_1',
      scan: { status: 'failed', error: 'daemon down', fixable: 0, top: [] },
    }
    renderSection(214)
    expect(
      await screen.findByText(/The last scan failed: daemon down/),
    ).toBeTruthy()
  })

  it('says a clean scan is clean', () => {
    render(
      <VulnCountPills
        counts={{ critical: 0, high: 0, medium: 0, low: 0, unknown: 0 }}
      />,
    )
    expect(screen.getByText('No known vulnerabilities')).toBeTruthy()
  })
})
