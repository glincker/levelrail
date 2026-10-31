import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GetCertificateButton } from './GetCertificateButton'
import httpsEn from '../locales/en/https.json'
import type { CertPreflight, DomainCertStatus } from '../queries/domainCert'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['https'],
  defaultNS: 'https',
  resources: { en: { https: httpsEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response
}

const ready: CertPreflight = {
  domain: 'shop.dev',
  staging: false,
  email: 'me@shop.dev',
  ready: true,
  items: [
    { code: 'dns', state: 'pass', detail: 'The domain points at this server.' },
    {
      code: 'email',
      state: 'pass',
      detail: 'Expiry notices go to me@shop.dev.',
    },
  ],
}

const blocked: CertPreflight = {
  ...ready,
  ready: false,
  items: [
    {
      code: 'caa',
      state: 'fail',
      detail: 'CAA blocks Lets Encrypt',
      fix: 'Add a CAA record.',
    },
  ],
}

const portsOnly: CertPreflight = {
  ...ready,
  ready: false,
  items: [{ code: 'ports', state: 'fail', detail: 'Port 80 did not answer.' }],
}

type Handler = (url: string, init?: RequestInit) => Response

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

function bodyOf(init?: RequestInit): unknown {
  return typeof init?.body === 'string' ? JSON.parse(init.body) : undefined
}

function stub(handler: Handler) {
  const fetchMock = vi.fn<
    (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>
  >((input, init) => Promise.resolve(handler(urlOf(input), init)))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderButton() {
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <GetCertificateButton domain="shop.dev" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

async function open() {
  const user = userEvent.setup()
  renderButton()
  await user.click(
    screen.getByRole('button', { name: 'Get a real certificate' }),
  )
  return user
}

function submitButton() {
  return screen
    .getAllByRole('button', { name: 'Get a real certificate' })
    .pop()!
}

describe('GetCertificateButton', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('runs the preflight and enables the action only when nothing is blocked', async () => {
    stub(() => json(ready))
    await open()
    expect(await screen.findByText('DNS points here')).toBeInTheDocument()
    await waitFor(() => expect(submitButton()).toBeEnabled())
    expect(screen.getByLabelText('Email for certificate notices')).toHaveValue(
      'me@shop.dev',
    )
  })

  it('blocks on a failed item and shows the next step', async () => {
    stub(() => json(blocked))
    await open()
    expect(
      await screen.findByText('Next step: Add a CAA record.'),
    ).toBeInTheDocument()
    expect(submitButton()).toBeDisabled()
  })

  it('lets the operator acknowledge a port 80 self-check failure', async () => {
    const fetchMock = stub((url, init) =>
      init?.method === 'POST'
        ? json(
            {
              domain: 'shop.dev',
              state: 'pending',
              staging: false,
              kind: 'none',
              applies_to_all: true,
            } satisfies DomainCertStatus,
            202,
          )
        : url.includes('/status')
          ? json({
              domain: 'shop.dev',
              state: 'pending',
              staging: false,
              kind: 'none',
              applies_to_all: true,
            })
          : json(portsOnly),
    )
    const user = await open()
    await screen.findByText('Port 80 reachable')
    expect(submitButton()).toBeDisabled()
    await user.click(
      screen.getByRole('checkbox', {
        name: /I know ports 80 and 443 are open/,
      }),
    )
    await user.click(submitButton())
    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        ([, init]) => init?.method === 'POST',
      )
      expect(post).toBeDefined()
      expect(bodyOf(post?.[1])).toMatchObject({
        acknowledge: ['ports'],
        staging: false,
      })
    })
    expect(await screen.findByText('Challenge pending')).toBeInTheDocument()
  })

  it('warns that staging is untrusted and sends the flag', async () => {
    const fetchMock = stub((url, init) =>
      init?.method === 'POST'
        ? json(
            {
              domain: 'shop.dev',
              state: 'issued',
              staging: true,
              kind: 'staging',
              applies_to_all: true,
              issuer: '(STAGING) Ersatz',
              not_after: '2027-01-01T00:00:00Z',
              days_left: 80,
            },
            202,
          )
        : url.includes('/status')
          ? json({
              domain: 'shop.dev',
              state: 'issued',
              staging: true,
              kind: 'staging',
              applies_to_all: true,
              issuer: '(STAGING) Ersatz',
              not_after: '2027-01-01T00:00:00Z',
              days_left: 80,
            })
          : json({ ...ready, staging: url.includes('staging=true') }),
    )
    const user = await open()
    await screen.findByText('DNS points here')
    await user.click(
      screen.getByRole('checkbox', {
        name: /Rehearse with Let's Encrypt staging/,
      }),
    )
    expect(
      screen.getByText(/Staging certificates are untrusted/),
    ).toBeInTheDocument()
    await waitFor(() => expect(submitButton()).toBeEnabled())
    await user.click(submitButton())
    expect(await screen.findByText('Certificate issued')).toBeInTheDocument()
    expect(screen.getByText('(STAGING) Ersatz')).toBeInTheDocument()
    expect(
      screen.getByText(/staging certificate, so browsers still show a warning/),
    ).toBeInTheDocument()
    const post = fetchMock.mock.calls.find(
      ([, init]) => init?.method === 'POST',
    )
    expect(bodyOf(post?.[1])).toMatchObject({ staging: true })
  })

  it('shows the issued certificate details with the renewal date', async () => {
    stub((url, init) => {
      const issued: DomainCertStatus = {
        domain: 'shop.dev',
        state: 'issued',
        staging: false,
        kind: 'letsencrypt',
        applies_to_all: true,
        issuer: 'R11',
        not_after: '2027-01-01T00:00:00Z',
        days_left: 89,
        renewal_at: '2026-12-01T00:00:00Z',
      }
      if (init?.method === 'POST') return json(issued, 202)
      return url.includes('/status') ? json(issued) : json(ready)
    })
    const user = await open()
    await screen.findByText('DNS points here')
    await waitFor(() => expect(submitButton()).toBeEnabled())
    await user.click(submitButton())
    expect(await screen.findByText('R11')).toBeInTheDocument()
    expect(screen.getByText('Renews from')).toBeInTheDocument()
    expect(screen.getByText(/89 days left/)).toBeInTheDocument()
  })

  it('explains a failed issuance in plain language and offers another try', async () => {
    const failed: DomainCertStatus = {
      domain: 'shop.dev',
      state: 'failed',
      staging: false,
      kind: 'none',
      applies_to_all: true,
      error: 'Timeout during connect',
      explanation:
        "Let's Encrypt could not reach this server on port 80 to check that you own the domain (the challenge failed).",
      next_safe_at: '2026-10-21T00:00:00Z',
    }
    stub((url, init) => {
      if (init?.method === 'POST') return json(failed, 202)
      return url.includes('/status') ? json(failed) : json(ready)
    })
    const user = await open()
    await screen.findByText('DNS points here')
    await waitFor(() => expect(submitButton()).toBeEnabled())
    await user.click(submitButton())
    expect(
      await screen.findByText("Let's Encrypt did not issue the certificate"),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/could not reach this server on port 80/),
    ).toBeInTheDocument()
    expect(screen.getByText('Timeout during connect')).toBeInTheDocument()
    expect(screen.getByText(/Next safe time to try/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('DNS points here')).toBeInTheDocument()
  })

  it('shows the server error when the request is refused', async () => {
    stub((_url, init) =>
      init?.method === 'POST'
        ? json({ error: 'too many certificate attempts' }, 409)
        : json(ready),
    )
    const user = await open()
    await screen.findByText('DNS points here')
    await waitFor(() => expect(submitButton()).toBeEnabled())
    await user.click(submitButton())
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not request the certificate.',
    )
  })

  it('blocks staging over a working certificate and shows the reason with its next step', async () => {
    const msg =
      'Staging would replace the working three certificate for shop.dev. Next step: request the real certificate without staging.'
    stub((url) =>
      json(
        url.includes('staging=true')
          ? {
              ...ready,
              staging: true,
              ready: false,
              items: [
                {
                  code: 'replaces_certificate',
                  state: 'fail',
                  detail: msg,
                  fix: 'Request the real certificate without staging.',
                },
              ],
            }
          : ready,
      ),
    )
    const user = await open()
    await screen.findByText('DNS points here')
    await user.click(
      screen.getByRole('checkbox', {
        name: /Rehearse with Let's Encrypt staging/,
      }),
    )
    expect(await screen.findByText(msg)).toBeInTheDocument()
    expect(
      screen.getByText(
        'Next step: Request the real certificate without staging.',
      ),
    ).toBeInTheDocument()
    expect(submitButton()).toBeDisabled()
  })
})
