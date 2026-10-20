import { screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainTLSCertControl } from './DomainTLSCertControl'
import {
  jsonResponse,
  renderWithProviders,
  stubFetch,
} from '../test/proxyTestUtils'

const URL = 'GET /api/v1/apps/web/domains/app.example.com/tls-cert'

describe('DomainTLSCertControl applied state', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows an uploaded certificate as pending until ingress loads it', async () => {
    stubFetch({
      [URL]: () =>
        jsonResponse({
          domain: 'app.example.com',
          enabled: true,
          expires_at: '2027-01-01T00:00:00Z',
          applied: false,
        }),
    })
    renderWithProviders(
      <DomainTLSCertControl appName="web" domain="app.example.com" />,
    )
    expect(await screen.findByText('Pending')).toBeInTheDocument()
    expect(screen.queryByText('In use')).not.toBeInTheDocument()
  })

  it('shows in use once applied', async () => {
    stubFetch({
      [URL]: () =>
        jsonResponse({
          domain: 'app.example.com',
          enabled: true,
          applied: true,
        }),
    })
    renderWithProviders(
      <DomainTLSCertControl appName="web" domain="app.example.com" />,
    )
    expect(await screen.findByText('In use')).toBeInTheDocument()
  })

  it('explains why a stored certificate is not in use', async () => {
    stubFetch({
      [URL]: () =>
        jsonResponse({
          domain: 'app.example.com',
          enabled: true,
          applied: false,
          apply_issue: 'key unreadable',
        }),
    })
    renderWithProviders(
      <DomainTLSCertControl appName="web" domain="app.example.com" />,
    )
    expect(await screen.findByText(/key unreadable/)).toBeInTheDocument()
  })
})
