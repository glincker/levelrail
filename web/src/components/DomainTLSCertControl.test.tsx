import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainTLSCertControl } from './DomainTLSCertControl'
import {
  jsonResponse,
  renderWithProviders,
  stubFetch,
} from '../test/proxyTestUtils'

describe('DomainTLSCertControl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('is collapsed by default and expands on demand', async () => {
    const user = userEvent.setup()
    stubFetch({
      'GET /api/v1/apps/web/domains/app.example.com/tls-cert': () =>
        jsonResponse({ domain: 'app.example.com', enabled: false }),
    })
    renderWithProviders(
      <DomainTLSCertControl appName="web" domain="app.example.com" />,
    )
    const toggle = await screen.findByRole('button', {
      name: /I have my own certificate/,
    })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(
      screen.getByText(
        'Only needed if you bought a certificate. Otherwise certificates are issued automatically.',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByLabelText('Certificate (PEM)')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Private key (PEM)')).not.toBeInTheDocument()

    await user.click(toggle)
    expect(screen.getByLabelText('Certificate (PEM)')).toBeInTheDocument()
    expect(screen.getByLabelText('Private key (PEM)')).toBeInTheDocument()
  })
})
