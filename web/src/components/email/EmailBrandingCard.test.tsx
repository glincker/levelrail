import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EmailBrandingCard } from './EmailBrandingCard'
import settingsEn from '../../locales/en/settings.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function res(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const saved = {
  logo_url: 'https://cdn.example.com/logo.png',
  brand_color: '#112233',
  footer_text: 'hello',
  footer_address: '',
  footer_links: [{ label: 'Privacy', url: 'https://example.com/privacy' }],
  show_powered_by: false,
  max_footer_links: 5,
  variables: ['brand_name'],
}

afterEach(() => vi.unstubAllGlobals())

describe('EmailBrandingCard', () => {
  it('loads saved branding and previews in an iframe with no script permission', async () => {
    const fetchMock = vi.fn((url: string) =>
      Promise.resolve(
        url.endsWith('/preview')
          ? res({ subject: 's', html: '<p>sample</p>', text: 't' })
          : res(saved),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={testI18n}>
          <EmailBrandingCard />
        </I18nextProvider>
      </QueryClientProvider>,
    )

    expect(await screen.findByDisplayValue(saved.logo_url)).toBeTruthy()
    const frame = await screen.findByTitle('Email branding preview')
    await waitFor(() =>
      expect(frame.getAttribute('srcdoc')).toBe('<p>sample</p>'),
    )
    expect(frame.getAttribute('sandbox')).toBe('')
    expect(frame.getAttribute('sandbox')).not.toContain('allow-scripts')
  })
})
