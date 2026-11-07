import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ProxyDomainStatus, ProxyHandledTlsBadge } from './ProxyDomainStatus'
import {
  jsonResponse,
  proxyDomain,
  proxyIntegration,
  renderWithProviders,
  stubFetch,
} from '../test/proxyTestUtils'

const BASE = '/api/v1/system/proxy-integration'

describe('ProxyHandledTlsBadge', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('replaces Provisioning with Handled by your proxy', async () => {
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(proxyIntegration()) })
    renderWithProviders(
      <ProxyHandledTlsBadge domain="app.example.com" fallback="Provisioning" />,
    )
    expect(await screen.findByText('Route not wired')).toBeInTheDocument()
    expect(screen.queryByText('Provisioning')).not.toBeInTheDocument()
  })

  it('shows the live certificate line when the contract has one', async () => {
    const data = proxyIntegration({ domains: [proxyDomain()] })
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(data) })
    renderWithProviders(
      <ProxyHandledTlsBadge domain="app.example.com" fallback="Provisioning" />,
    )
    expect(
      await screen.findByText(/Let's Encrypt, valid until/),
    ).toBeInTheDocument()
  })

  it('keeps the fallback when TLS is not terminated upstream', async () => {
    const data = proxyIntegration()
    data.ingress.tls_terminated_upstream = false
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(data) })
    renderWithProviders(
      <ProxyHandledTlsBadge domain="app.example.com" fallback="Provisioning" />,
    )
    expect(await screen.findByText('Provisioning')).toBeInTheDocument()
    expect(screen.queryByText('Handled by your proxy')).not.toBeInTheDocument()
  })
})

describe('ProxyDomainStatus recheck', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('has no duplicate Recheck DNS button, Check now lives in the DNS card', async () => {
    const data = proxyIntegration({ domains: [proxyDomain()] })
    stubFetch({ [`GET ${BASE}`]: () => jsonResponse(data) })
    renderWithProviders(
      <ProxyDomainStatus domain="app.example.com" appName="web" />,
    )
    await new Promise((r) => setTimeout(r, 50))
    expect(
      screen.queryByRole('button', { name: 'Recheck DNS' }),
    ).not.toBeInTheDocument()
  })
})
