import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import getStartedEn from '../../locales/en/getStarted.json'
import { SampleAppButton } from './SampleAppButton'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['getStarted'],
  defaultNS: 'getStarted',
  resources: { en: { getStarted: getStartedEn } },
  interpolation: { escapeValue: false },
})

function renderButton() {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient()}>
        <SampleAppButton />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('SampleAppButton', () => {
  it('creates the sample app and then offers to open it', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ name: 'sample-app' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderButton()
    fireEvent.click(screen.getByRole('button', { name: 'Deploy a sample app' }))
    await waitFor(() => {
      expect(screen.getByText('Open sample app')).toBeTruthy()
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
