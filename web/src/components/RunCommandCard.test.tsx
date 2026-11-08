import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import appRunEn from '../locales/en/appRun.json'
import { RunCommandCard } from './RunCommandCard'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['appRun'],
  defaultNS: 'appRun',
  resources: { en: { appRun: appRunEn } },
  interpolation: { escapeValue: false },
})

function renderCard() {
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <RunCommandCard name="shop" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

describe('RunCommandCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('keeps Run disabled for an empty or whitespace command', async () => {
    const user = userEvent.setup()
    renderCard()
    const run = screen.getByRole('button', { name: 'Run' })
    expect(run).toBeDisabled()
    await user.type(screen.getByLabelText('Command'), '   ')
    expect(run).toBeDisabled()
  })

  it('posts argv with backoff_limit 0 and shows output and a failed exit code', async () => {
    const user = userEvent.setup()
    const fetchMock = vi.fn((url: string) =>
      url.endsWith('/jobs/runs')
        ? json({ id: 'j1', app: 'shop', state: 'Running' }, 202)
        : json({
            id: 'j1',
            app: 'shop',
            state: 'Failed',
            exit_code: 2,
            output: 'boom',
            output_truncated: true,
          }),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderCard()

    await user.type(screen.getByLabelText('Command'), 'bin/migrate --force')
    await user.click(screen.getByRole('button', { name: 'Run' }))

    expect(
      await screen.findByText('boom', {}, { timeout: 4000 }),
    ).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
    expect(screen.getByText(/only the last part is shown/)).toBeInTheDocument()
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(JSON.parse(init.body as string)).toEqual({
      app: 'shop',
      command: ['bin/migrate', '--force'],
      backoff_limit: 0,
    })
  })

  it('shows a succeeded run and the server error text inline', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        json(
          {
            id: 'j2',
            app: 'shop',
            state: 'Succeeded',
            exit_code: 0,
            output: 'ok',
          },
          202,
        ),
      ),
    )
    renderCard()
    await user.type(screen.getByLabelText('Command'), 'true')
    await user.click(screen.getByRole('button', { name: 'Run' }))
    await waitFor(() => expect(screen.getByText('ok')).toBeInTheDocument())
    expect(screen.getByText('0')).toBeInTheDocument()

    vi.stubGlobal(
      'fetch',
      vi.fn(() => json({ error: 'no permission to run here' }, 403)),
    )
    await user.click(screen.getByRole('button', { name: 'Run' }))
    expect(
      await screen.findByText(/follow console exec access rules/),
    ).toBeInTheDocument()
  })
})
