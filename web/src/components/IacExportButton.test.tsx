import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import settingsEn from '../locales/en/settings.json'
import { IacExportButton } from './IacExportButton'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

describe('IacExportButton', () => {
  it('requests the one resource and shows its YAML', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          files: [{ name: 'web.yaml', kind: 'App', content: 'kind: App\n' }],
        }),
        { status: 200 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={new QueryClient()}>
        <IacExportButton kind="app" name="web" />
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button'))
    await waitFor(() =>
      expect(screen.getByLabelText(/exported resource files/i)).toBeTruthy(),
    )
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('app=web')
    vi.unstubAllGlobals()
  })
})
