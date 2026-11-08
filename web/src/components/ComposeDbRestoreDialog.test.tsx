import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { ComposeDbRestoreDialog } from './ComposeDbRestoreDialog'
import backupsEn from '../locales/en/backups.json'
import commonEn from '../locales/en/common.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'backups'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, backups: backupsEn } },
  interpolation: { escapeValue: false },
})

const PHRASE = 'restore shop/db'

function json(body: unknown, status = 200): Promise<Response> {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

function bodyOf(init?: RequestInit): Record<string, unknown> {
  return typeof init?.body === 'string'
    ? (JSON.parse(init.body) as Record<string, unknown>)
    : {}
}

describe('ComposeDbRestoreDialog', () => {
  it('previews on open and confirms only with the exact server phrase', async () => {
    const calls: Record<string, unknown>[] = []
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      const body = bodyOf(init)
      calls.push(body)
      const base = {
        run_id: 'r1',
        object_key: 'k/r1.sql.gz',
        size_bytes: 10,
        engine: 'postgres',
        app: 'shop',
        service: 'db',
        confirm_phrase: PHRASE,
        preview: body.preview,
      }
      return body.preview
        ? json(base)
        : json({ ...base, safety_run_id: 's9' }, 202)
    })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <ComposeDbRestoreDialog
            app="shop"
            service="db"
            run="r1"
            onClose={() => undefined}
          />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    const input = await screen.findByLabelText(/Type restore shop\/db/)
    expect(calls[0]).toEqual({ confirm: '', preview: true })
    const button = screen.getByRole('button', { name: 'Replace database' })
    expect(button).toBeDisabled()
    fireEvent.change(input, { target: { value: 'restore shop/d' } })
    expect(button).toBeDisabled()
    fireEvent.change(input, { target: { value: PHRASE } })
    expect(button).toBeEnabled()
    fireEvent.click(button)
    await waitFor(() =>
      expect(calls[1]).toEqual({ confirm: PHRASE, preview: false }),
    )
    expect(await screen.findByText(/Safety dump: s9/)).toBeInTheDocument()
  })
})
