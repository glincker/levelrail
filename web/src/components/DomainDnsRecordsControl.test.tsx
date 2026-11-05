import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { ConnectNsProvider } from './DomainDnsRecordsControl'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: { provider?: string }) =>
      o?.provider ? `${key}:${o.provider}` : key,
  }),
}))

function renderStrip(provider?: string) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <ConnectNsProvider provider={provider} />
    </QueryClientProvider>,
  )
}

describe('ConnectNsProvider', () => {
  it('offers Connect Cloudflare for Cloudflare name servers', () => {
    renderStrip('cloudflare')
    expect(
      screen.getByRole('button', { name: 'recordsControl.connect:Cloudflare' }),
    ).toBeInTheDocument()
  })

  it('offers Connect Route53 for AWS name servers', () => {
    renderStrip('route53')
    expect(
      screen.getByRole('button', { name: 'recordsControl.connect:Route53' }),
    ).toBeInTheDocument()
  })

  it.each(['other', 'unknown', undefined])('renders nothing for %s', (p) => {
    renderStrip(p)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})

describe('DomainDnsRecordsControl', () => {
  it('shows the record count without any click', async () => {
    const { DomainDnsRecordsControl } =
      await import('./DomainDnsRecordsControl')
    render(
      <QueryClientProvider client={new QueryClient()}>
        <DomainDnsRecordsControl appName="a" domain="d.test" />
      </QueryClientProvider>,
    )
    expect(screen.getByText('recordsControl.title')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'recordsControl.manage' }),
    ).not.toBeInTheDocument()
  })
})
