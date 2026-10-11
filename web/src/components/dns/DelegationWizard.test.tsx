import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DelegationWizard } from './DelegationWizard'
import { jsonResponse, renderDns } from './dnsTestUtils'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

const zone = {
  id: 'z1',
  name: 'example.com',
  provider: 'cloudflare',
  name_servers: ['ana.ns.cloudflare.com', 'bob.ns.cloudflare.com'],
}

function currentStep() {
  return document.querySelector('[aria-current="step"]')?.textContent ?? ''
}

describe('DelegationWizard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('walks domain, name servers, import and verify', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url === '/api/v1/dns/zones' && init?.method === 'POST') {
        return Promise.resolve(jsonResponse(zone, 201))
      }
      if (url.includes('/discover')) {
        return Promise.resolve(
          jsonResponse({
            servers: ['ns1.registrar.net'],
            records: [
              { name: 'www', type: 'A', ttl: 300, values: ['203.0.113.1'] },
            ],
            plan: { changes: [], summary: {}, blocked: false },
          }),
        )
      }
      if (url.includes('/records/import')) {
        return Promise.resolve(
          jsonResponse({
            plan: { changes: [], summary: {}, blocked: false },
            applied: 1,
          }),
        )
      }
      if (url.includes('/delegation')) {
        return Promise.resolve(
          jsonResponse({
            delegation: {
              domain: 'example.com',
              state: 'delegated_elsewhere',
              expected: zone.name_servers,
              resolvers: [
                {
                  server: '1.1.1.1:53',
                  verdict: 'elsewhere',
                  name_servers: ['ns1.registrar.net'],
                },
              ],
              elsewhere: ['ns1.registrar.net'],
            },
            checked_at: '2026-10-10T00:00:00Z',
          }),
        )
      }
      return Promise.resolve(jsonResponse({}, 404))
    })
    vi.stubGlobal('fetch', fetchMock)
    renderDns(<DelegationWizard open onOpenChange={() => undefined} />)

    expect(currentStep()).toContain('Domain')
    const create = screen.getByRole('button', { name: 'Create zone' })
    await userEvent.type(screen.getByLabelText('Domain'), 'not a domain')
    expect(create).toBeDisabled()
    await userEvent.clear(screen.getByLabelText('Domain'))
    await userEvent.type(screen.getByLabelText('Domain'), 'Example.com')
    await userEvent.click(create)

    await waitFor(() => expect(currentStep()).toContain('Name servers'))
    expect(screen.getByText('ana.ns.cloudflare.com')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/dns/zones',
      expect.objectContaining({
        body: JSON.stringify({ name: 'example.com' }),
      }),
    )

    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(currentStep()).toContain('Import records')
    await screen.findByText('203.0.113.1')
    await userEvent.click(screen.getByRole('button', { name: 'Import 1 set' }))

    await waitFor(() => expect(currentStep()).toContain('Verify'))
    expect(await screen.findByText('Delegated elsewhere')).toBeInTheDocument()
    expect(
      screen.getByText('Still pointing at: ns1.registrar.net'),
    ).toBeInTheDocument()
  })

  it('starts at the name servers step for an existing zone', () => {
    vi.stubGlobal('fetch', vi.fn())
    renderDns(
      <DelegationWizard open existing={zone} onOpenChange={() => undefined} />,
    )
    expect(currentStep()).toContain('Name servers')
    expect(screen.getByText('bob.ns.cloudflare.com')).toBeInTheDocument()
  })
})
