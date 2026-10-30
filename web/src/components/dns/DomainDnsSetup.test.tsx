import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/dnsSetup.json'
import { DomainDnsSetup } from './DomainDnsSetup'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { dnsSetup: en } },
  defaultNS: 'dnsSetup',
  interpolation: { escapeValue: false },
})

const detection = (o: object) => ({
  domain: 'example.com',
  apex: 'example.com',
  zone: 'example.com',
  nameservers: [],
  provider: 'cloudflare',
  domain_connect: { supported: false },
  connected: true,
  connectable: true,
  can_issue_certificates: false,
  hint: '',
  ...o,
})
const rec = (
  kind: string,
  type: string,
  action: string,
  extra: object = {},
) => ({
  action,
  kind,
  set: { name: `${kind}.example.com`, type, ttl: 300, values: ['v'] },
  ...extra,
})

function mockApi(
  det: object,
  changes: object[],
  onBody?: (b: unknown) => void,
) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const text = typeof init?.body === 'string' ? init.body : ''
      const json = (b: unknown) =>
        new Response(JSON.stringify(b), { status: 200 })
      if (url.includes('dns-provider')) return json(det)
      if (url.includes('/mail/dns/domains'))
        return json({
          domains: [
            { domain: 'example.com', source: 'mail', sources: ['mail'] },
          ],
        })
      if (url.includes('/server-address')) return json({ ipv4: '203.0.113.7' })
      if (url.includes('/mail/dns/plan')) {
        const b = JSON.parse(text) as {
          replace_existing?: string[]
        }
        const out = b.replace_existing?.length
          ? (changes as { action: string }[]).map((c) =>
              c.action === 'refused' ? { ...c, action: 'replace' } : c,
            )
          : changes
        return json({
          domain: 'example.com',
          hash: 'h1',
          changes: out,
          summary: {},
          blocked: false,
        })
      }
      if (url.includes('/mail/dns/apply')) {
        onBody?.(JSON.parse(text))
        return json({
          applied: 1,
          plan: {
            domain: 'example.com',
            hash: 'h1',
            changes,
            summary: {},
            blocked: false,
          },
        })
      }
      if (url.includes('/mail/dns/check'))
        return json({ domain: 'example.com', items: [] })
      return json({})
    }),
  )
}

function setup(
  props: Partial<React.ComponentProps<typeof DomainDnsSetup>> = {},
) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <DomainDnsSetup kind="mail" domain="example.com" {...props} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}
afterEach(() => vi.unstubAllGlobals())

describe('DomainDnsSetup', () => {
  it('says connected and offers Add N records, with host line prefilled', async () => {
    mockApi(detection({}), [rec('MX', 'MX', 'add'), rec('SPF', 'TXT', 'add')])
    setup()
    expect(
      await screen.findByText('DNS is hosted at Cloudflare. Connected.'),
    ).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(
      await screen.findByRole('button', { name: 'Add 2 records' }),
    ).toBeTruthy()
    expect(screen.getByTestId('host-line').textContent).toContain(
      'mail.example.com -> 203.0.113.7',
    )
    expect(screen.getByText('Nothing is deleted.')).toBeTruthy()
  })
  it('not connected shows Connect and never echoes a token', async () => {
    mockApi(detection({ connected: false }), [rec('MX', 'MX', 'add')])
    setup()
    expect(
      await screen.findByText('DNS is hosted at Cloudflare. Not connected.'),
    ).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Continue without connecting' }),
    )
    expect(
      await screen.findByRole('button', { name: 'Connect Cloudflare' }),
    ).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Connect Cloudflare' }),
    )
    const input = await screen.findByLabelText('Cloudflare API token')
    expect(input.getAttribute('type')).toBe('password')
    await userEvent.type(input, 'secret-token-123')
    expect(document.body.textContent).not.toContain('secret-token-123')
  })
  it('unknown host offers Copy records', async () => {
    mockApi(detection({ provider: '', connected: false, connectable: false }), [
      rec('MX', 'MX', 'add'),
    ])
    setup()
    expect(
      await screen.findByText(/could not detect your DNS host/),
    ).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
    expect(
      await screen.findByRole('button', { name: 'Copy records' }),
    ).toBeTruthy()
  })
  it('Different offers Replace and needs a second confirm', async () => {
    const body = vi.fn()
    const before = { name: 'x', type: 'TXT', ttl: 300, values: ['old'] }
    mockApi(
      detection({}),
      [rec('SPF', 'TXT', 'refused', { before }), rec('MX', 'MX', 'add')],
      body,
    )
    setup()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Continue' }),
    )
    expect(
      await screen.findByRole('button', { name: 'Add 1, review 1 different' }),
    ).toBeTruthy()
    expect(screen.getByText('Found: old')).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Existing record' }),
    )
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Replace' }),
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Add 2 records' }),
    )
    expect(body).not.toHaveBeenCalled()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Yes, replace 1 and add' }),
    )
    await waitFor(() => expect(body).toHaveBeenCalled())
    expect(body.mock.calls[0]?.[0]).toMatchObject({
      replace_existing: ['SPF'],
      plan_hash: 'h1',
    })
  })
  const toReview = async (det: object) => {
    mockApi(det, [rec('MX', 'MX', 'add')])
    setup()
    await screen.findByTestId('detect-sentence')
    const btn =
      screen.queryByRole('button', { name: 'Continue without connecting' }) ??
      screen.getByRole('button', { name: 'Continue' })
    await userEvent.click(btn)
    await screen.findByTestId('detect-banner')
  }
  it('offers no approve card when the flow flag is absent', async () => {
    await toReview(
      detection({ connected: false, domain_connect: { supported: true } }),
    )
    await screen.findByRole('button', { name: 'Connect Cloudflare' })
    expect(screen.queryByTestId('apply-options')).toBeNull()
    expect(screen.queryByText(/no token/)).toBeNull()
  })
  it('shows approve and token cards side by side when the flow flag is set', async () => {
    await toReview(
      detection({
        connected: false,
        domain_connect: { supported: true, flow: true, host: 'Cloudflare' },
      }),
    )
    const group = await screen.findByTestId('apply-options')
    expect(group.textContent).toContain('Approve at Cloudflare (no token)')
    expect(group.textContent).toContain('Connect Cloudflare with a token')
  })
  it('connected shows Add N records and no option cards', async () => {
    await toReview(
      detection({ domain_connect: { supported: true, flow: true } }),
    )
    expect(
      await screen.findByRole('button', { name: 'Add 1 record' }),
    ).toBeTruthy()
    expect(screen.queryByTestId('apply-options')).toBeNull()
  })
  it('renders when detection has no domain_connect field', async () => {
    const det = detection({}) as Record<string, unknown>
    delete det.domain_connect
    await toReview(det)
    expect(
      await screen.findByRole('button', { name: 'Add 1 record' }),
    ).toBeTruthy()
  })
})
