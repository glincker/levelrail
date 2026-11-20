import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { DomainsStatusBar } from './DomainsStatusBar'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('../../hooks/useCopyToClipboard', () => ({
  useCopyToClipboard: () => ({ copied: false, copy: vi.fn() }),
}))
vi.mock('../../queries/serverAddress', () => ({
  useServerAddress: () => ({ data: { ipv4: '203.0.113.7' } }),
}))
vi.mock('../../queries/parkedDomains', () => ({
  useParkedDomains: () => ({ data: [] }),
}))
vi.mock('../../queries/proxyIntegration', () => ({
  useProxyIntegration: () => ({ data: { detected: { kind: 'none' } } }),
}))
vi.mock('../platform/usePlatformStatus', () => ({
  usePlatformStatus: () => ({ httpsOff: false }),
}))
vi.mock('../ProxySetupCard', () => ({ ProxySetupCard: () => null }))
vi.mock('../getStarted/SetupStepHint', () => ({ SetupStepHint: () => null }))
vi.mock('./ServerAddressCard', () => ({ ServerAddressCard: () => null }))
vi.mock('./ReachabilityPill', () => ({ ReachabilityPill: () => null }))

describe('DomainsStatusBar', () => {
  it('gives each status item an info hover in one row', () => {
    render(<DomainsStatusBar onAdd={vi.fn()} />)
    const region = screen.getByRole('region', { name: 'statusBar.label' })
    for (const label of [
      'statusBar.addressLabel',
      'statusBar.httpsLabel',
      'statusBar.portsLabel',
    ]) {
      expect(
        region.querySelector(`button[aria-label="${label}"]`),
      ).not.toBeNull()
    }
  })
})
