import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AppVolumeFilesSection } from './AppVolumeFilesSection'
import backupsEn from '../locales/en/backups.json'
import type { AppVolume } from '../types/appDetail'

const volumes = [{ name: 'data' }] as unknown as AppVolume[]

function renderSection() {
  const i18n = i18next.createInstance()
  void i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { backups: backupsEn } },
    interpolation: { escapeValue: false },
  })
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <AppVolumeFilesSection appName="web" volumes={volumes} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AppVolumeFilesSection', () => {
  it('lists entries and surfaces the server error on upload', async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      if (init?.method === 'PUT') {
        return new Response(JSON.stringify({ error: 'file too large' }), {
          status: 413,
          headers: { 'Content-Type': 'application/json' },
        })
      }
      return new Response(
        JSON.stringify({
          path: '',
          truncated: false,
          entries: [
            {
              name: 'a.txt',
              path: 'a.txt',
              type: 'file',
              size: 3,
              mode: '0644',
              mod_time: '2026-01-01T00:00:00Z',
            },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } },
      )
    })
    vi.stubGlobal('fetch', fetchMock)
    renderSection()
    expect(await screen.findByText('a.txt')).toBeTruthy()

    const input = document.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement
    await userEvent.upload(input, new File(['x'], 'big.bin'))
    await waitFor(() => expect(screen.getByRole('alert')).toBeTruthy())
    expect(screen.getByRole('alert').textContent).toContain('file too large')
  })
})
