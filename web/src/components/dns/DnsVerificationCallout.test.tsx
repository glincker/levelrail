import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DnsVerificationCallout } from './DnsVerificationCallout'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: { count?: number }) =>
      o?.count === undefined ? key : `${key}:${o.count}`,
  }),
}))

describe('DnsVerificationCallout', () => {
  it('renders nothing without a verification', () => {
    const { container } = render(<DnsVerificationCallout />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the visible zone count when ok', () => {
    render(
      <DnsVerificationCallout
        verification={{ ok: true, zones_visible: 3, missing_scopes: [] }}
      />,
    )
    expect(screen.getByRole('status')).toHaveTextContent('verification.ok:3')
  })

  it('lists missing scopes, the message and the fix guidance', () => {
    render(
      <DnsVerificationCallout
        verification={{
          ok: false,
          missing_scopes: ['Zone:DNS:Edit', 'Zone:Zone:Read'],
          message: 'Cloudflare rejected the token.',
        }}
      />,
    )
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Zone:DNS:Edit')
    expect(alert).toHaveTextContent('Zone:Zone:Read')
    expect(alert).toHaveTextContent('Cloudflare rejected the token.')
    expect(alert).toHaveTextContent('verification.fix')
  })

  it('copes with absent optional fields on failure', () => {
    render(<DnsVerificationCallout verification={{}} />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'verification.failedTitle',
    )
  })
})
