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

  it('sends the node filter for a node export', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ files: [] }), { status: 200 }),
      )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={new QueryClient()}>
        <IacExportButton kind="node" name="worker-1" />
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button'))
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('node=worker-1')
    vi.unstubAllGlobals()
  })
})

describe('IacExportButton studio mode', () => {
  it('exports everything without env values and hands the YAML over', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          files: [
            { name: 'a.yaml', kind: 'App', content: 'kind: App\n' },
            { name: 'b.yaml', kind: 'App', content: 'kind: Database\n' },
          ],
        }),
        { status: 200 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const onExported = vi.fn()
    render(
      <QueryClientProvider client={new QueryClient()}>
        <IacExportButton onExported={onExported} />
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button'))
    await waitFor(() =>
      expect(onExported).toHaveBeenCalledWith(
        'kind: App\n---\nkind: Database\n',
      ),
    )
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain(
      'include_env_values=false',
    )
    vi.unstubAllGlobals()
  })
})
