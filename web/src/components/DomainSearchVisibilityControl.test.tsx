import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import domains from '../locales/en/domains.json'
import { DomainSearchVisibilityControl } from './DomainSearchVisibilityControl'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { domains } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderControl() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <DomainSearchVisibilityControl
          appName="demo"
          domain="console.example.com"
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('DomainSearchVisibilityControl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows open by default and hides the domain on click', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        json({ domain: 'console.example.com', hidden: false }),
      )
      .mockResolvedValueOnce(
        json({ domain: 'console.example.com', hidden: true }),
      )
    vi.stubGlobal('fetch', fetchMock)
    renderControl()

    expect(
      await screen.findByText('Open to search engines'),
    ).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Hide from search engines' }),
    )
    await waitFor(() =>
      expect(
        screen.getByText('Hidden from search engines'),
      ).toBeInTheDocument(),
    )
    const put = fetchMock.mock.calls[1] as [string, RequestInit]
    expect(put[1]).toMatchObject({ method: 'PUT', body: '{"hidden":true}' })
  })
})
