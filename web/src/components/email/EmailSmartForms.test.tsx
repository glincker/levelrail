import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BrandContext } from '../../lib/brandContext'
import type { Brand } from '../../types/brand'
import type { EmailSettings } from '../../queries/emailSettings'
import type { EmailProvider } from '../../queries/emailProviders'
import { EmailBrandingCard } from './EmailBrandingCard'
import { EmailDeliveryCard } from './EmailDeliveryCard'
import { EmailSenderCard } from './EmailSenderCard'
import { EmailStatusHeader } from './EmailStatusHeader'
import settingsEn from '../../locales/en/settings.json'
import commonEn from '../../locales/en/common.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['settings', 'common'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const brand = {
  Name: 'Acme Deploy',
  PrimaryColor: '#107292',
} as unknown as Brand

const emptyBranding = {
  logo_url: '',
  brand_color: '',
  footer_text: '',
  footer_address: '',
  footer_links: [],
  show_powered_by: false,
  max_footer_links: 5,
  variables: ['brand_name', 'support_email', 'instance_name', 'year'],
}

const settings: EmailSettings = { backend: '' }

interface Call {
  method: string
  path: string
  body: Record<string, unknown> | null
}

function json(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function setup(ui: ReactNode, providers: EmailProvider[] = []) {
  const calls: Call[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({
        method,
        path: url,
        body: init?.body
          ? (JSON.parse(init.body as string) as Record<string, unknown>)
          : null,
      })
      if (url.includes('/auth/session')) {
        return Promise.resolve(json({ username: 'owner@example.com' }))
      }
      if (url.includes('ingress-settings') || url.includes('/https')) {
        return Promise.resolve(
          json({ public_host: 'deploy.example.com', state: 'issued' }),
        )
      }
      if (url.endsWith('/branding/preview')) {
        return Promise.resolve(json({ subject: '', html: '', text: '' }))
      }
      if (url.endsWith('/branding')) return Promise.resolve(json(emptyBranding))
      if (url.endsWith('/providers'))
        return Promise.resolve(json({ providers }))
      return Promise.resolve(json(settings))
    }),
  )
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const user = userEvent.setup()
  const view = render(
    <I18nextProvider i18n={testI18n}>
      <BrandContext.Provider value={brand}>
        <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
      </BrandContext.Provider>
    </I18nextProvider>,
  )
  return { calls, user, view }
}

afterEach(() => vi.unstubAllGlobals())

describe('sender suggestions', () => {
  it('offers instance-derived values, applies one on click and saves nothing until Save', async () => {
    const { calls, user } = setup(<EmailSenderCard settings={settings} />)

    const chip = await screen.findByRole('button', {
      name: 'Use the suggested value noreply@deploy.example.com',
    })
    expect(screen.getByText('Suggested: Acme Deploy')).toBeTruthy()
    expect(await screen.findByText('Suggested: owner@example.com')).toBeTruthy()

    await user.click(chip)
    expect(screen.getByLabelText<HTMLInputElement>('From address').value).toBe(
      'noreply@deploy.example.com',
    )
    expect(calls.some((c) => c.method === 'PUT')).toBe(false)
    expect(
      screen.queryByRole('button', {
        name: 'Use the suggested value noreply@deploy.example.com',
      }),
    ).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(calls.some((c) => c.method === 'PUT')).toBe(true)
    })
    expect(calls.find((c) => c.method === 'PUT')?.body).toMatchObject({
      from_address: 'noreply@deploy.example.com',
    })
  })

  it('validates addresses as the user types', async () => {
    const { user } = setup(<EmailSenderCard settings={settings} />)
    await user.type(await screen.findByLabelText('Reply-to address'), 'nope')
    expect(await screen.findByText('Enter a valid email address')).toBeTruthy()
  })
})

describe('branding templates', () => {
  it('fills footer, link and brand color as unsaved edits', async () => {
    const { calls, user } = setup(<EmailBrandingCard />)
    await user.click(await screen.findByRole('button', { name: 'Standard' }))

    const footer = screen.getByLabelText<HTMLTextAreaElement>('Footer text')
    expect(footer.value).toContain('{{brand_name}}')
    expect(footer.value).toContain('{{instance_name}}')
    expect(screen.getByLabelText<HTMLInputElement>('Accent color').value).toBe(
      '#107292',
    )
    expect(screen.getByLabelText<HTMLInputElement>('https link').value).toBe(
      'https://deploy.example.com',
    )
    expect(calls.some((c) => c.method === 'PUT')).toBe(false)
  })

  it('flags a non-https logo URL while typing and blocks Save', async () => {
    const { user } = setup(<EmailBrandingCard />)
    await user.type(
      await screen.findByLabelText('Logo URL'),
      'http://x.test/a.png',
    )
    expect(
      (await screen.findAllByText(/Use a full https/)).length,
    ).toBeGreaterThan(0)
    expect(
      screen.getByRole<HTMLButtonElement>('button', { name: /Save/ }).disabled,
    ).toBe(true)
  })
})

describe('hidden until a provider exists', () => {
  it('renders no status bar and no delivery check without providers', async () => {
    const { view, calls } = setup(
      <>
        <EmailStatusHeader />
        <EmailDeliveryCard />
      </>,
    )
    await waitFor(() => {
      expect(calls.some((c) => c.path.endsWith('/providers'))).toBe(true)
    })
    expect(view.container.querySelector('[data-slot="card"]')).toBeNull()
  })

  it('shows the delivery check once a provider exists', async () => {
    setup(<EmailDeliveryCard />, [
      { id: 'p1', role: 'primary', backend: 'resend', name: 'Resend' },
    ])
    expect(await screen.findByText('Delivery check')).toBeTruthy()
  })
})
