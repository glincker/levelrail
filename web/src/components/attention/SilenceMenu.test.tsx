import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import attentionEn from '../../locales/en/attention.json'
import commonEn from '../../locales/en/common.json'
import { SilenceMenu } from './SilenceMenu'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['attention', 'common'],
  defaultNS: 'attention',
  resources: { en: { attention: attentionEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function renderMenu(severity: string) {
  const qc = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <SilenceMenu issueKey="doctor:disk" severity={severity} title="Disk" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

function mockFetch() {
  const fn = vi.fn(
    () =>
      new Response(JSON.stringify({ id: 'sil_1', issue_key: 'doctor:disk' }), {
        status: 201,
      }),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

describe('SilenceMenu', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('offers the five durations and posts the chosen one with the reason', async () => {
    const fetchFn = mockFetch()
    const user = userEvent.setup()
    renderMenu('warning')
    await user.click(screen.getByRole('button', { name: /Silence Disk/ }))
    for (const label of [
      'For 1 hour',
      'For 1 day',
      'For 1 week',
      'For 30 days',
      'Forever',
    ]) {
      expect(screen.getByRole('button', { name: label })).toBeInTheDocument()
    }
    await user.type(screen.getByLabelText('Reason for silencing'), 'dev box')
    await user.click(screen.getByRole('button', { name: 'For 1 day' }))
    await waitFor(() => {
      expect(fetchFn).toHaveBeenCalled()
    })
    const [url, init] = fetchFn.mock.calls[0] as unknown as [
      string,
      RequestInit,
    ]
    expect(url).toBe('/api/v1/silences')
    expect(JSON.parse(init.body as string)).toMatchObject({
      issue_key: 'doctor:disk',
      duration: '1d',
      reason: 'dev box',
    })
  })

  it('asks for confirmation before silencing forever', async () => {
    const fetchFn = mockFetch()
    const user = userEvent.setup()
    renderMenu('warning')
    await user.click(screen.getByRole('button', { name: /Silence Disk/ }))
    await user.click(screen.getByRole('button', { name: 'Forever' }))
    expect(await screen.findByText('Silence Disk?')).toBeInTheDocument()
    expect(fetchFn).not.toHaveBeenCalled()
  })
})
