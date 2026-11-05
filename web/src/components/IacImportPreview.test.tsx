import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import settingsEn from '../locales/en/settings.json'
import { IacImportPreview } from './IacImportPreview'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function setup() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <IacImportPreview name="web" />
    </QueryClientProvider>,
  )
}

async function openAndPreview() {
  await userEvent.click(screen.getByRole('button', { name: /import yaml/i }))
  await userEvent.type(screen.getByLabelText(/yaml to preview/i), 'kind: App')
  await userEvent.click(screen.getByRole('button', { name: /^preview$/i }))
}

describe('IacImportPreview', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('plans the pasted YAML and shows the CLI note without an apply button', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          plan: {
            changes: [{ kind: 'App', name: 'web', action: 'create' }],
            summary: { create: 1, update: 0, delete: 0, noop: 0, error: 0 },
            hash: 'h',
          },
        }),
        { status: 200 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    setup()
    await openAndPreview()
    await waitFor(() => expect(screen.getByText('App/web')).toBeTruthy())
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('/apply/plan')
    expect(screen.queryByRole('button', { name: /^apply/i })).toBeNull()
    expect(screen.getByText(/levelrail-cli apply -f/)).toBeTruthy()
  })

  it('renders plan errors inline', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ error: 'bad yaml' }), { status: 400 }),
        ),
    )
    setup()
    await openAndPreview()
    await waitFor(() => expect(screen.getByRole('alert')).toBeTruthy())
  })
})
