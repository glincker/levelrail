import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ProviderRecordGuide } from './ProviderRecordGuide'
import type { DomainCheckResult } from '../../queries/domainCheck'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: Record<string, string>) =>
      o ? `${key}|${Object.values(o).join(',')}` : key,
  }),
}))

function check(p: DomainCheckResult['nameserver_provider']) {
  return {
    domain: 'app.example.com',
    zone: 'example.com',
    expected_ipv4: ['203.0.113.7'],
    nameserver_provider: p,
  } as DomainCheckResult
}

describe('ProviderRecordGuide', () => {
  it('shows steps, host and value for a detected provider', () => {
    render(
      <ProviderRecordGuide check={check('hetzner')} domain="app.example.com" />,
    )
    expect(
      screen.getByText('providerGuide.where.hetzner|Hetzner'),
    ).toBeInTheDocument()
    expect(
      screen.getByText('providerGuide.record|app,203.0.113.7'),
    ).toBeInTheDocument()
    expect(screen.queryByText('providerGuide.proxyOff')).not.toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('mentions the proxy and links docs for Cloudflare', () => {
    render(
      <ProviderRecordGuide
        check={check('cloudflare')}
        domain="app.example.com"
      />,
    )
    expect(screen.getByText('providerGuide.proxyOff')).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      expect.stringContaining('developers.cloudflare.com'),
    )
  })

  it.each(['other', 'unknown', undefined] as const)('hides for %s', (p) => {
    const { container } = render(
      <ProviderRecordGuide check={check(p)} domain="app.example.com" />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})
