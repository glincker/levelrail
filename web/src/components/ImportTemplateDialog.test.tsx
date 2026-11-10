import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import templates from '../locales/en/templates.json'
import { ImportTemplateDialog } from './ImportTemplateDialog'

function setup() {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ id: 'my-app', name: 'My App' }), {
      status: 201,
    }),
  )
  vi.stubGlobal('fetch', fetchMock)
  const i18n = createInstance()
  void i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { templates } },
    defaultNS: 'templates',
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient()}>
        <ImportTemplateDialog />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return fetchMock
}

describe('ImportTemplateDialog', () => {
  it('derives the id from the name and posts the compose body', async () => {
    const user = userEvent.setup()
    const fetchMock = setup()
    await user.click(screen.getByRole('button', { name: /import compose/i }))
    await user.type(screen.getByLabelText(/^name$/i), 'My App')
    await user.type(screen.getByLabelText(/^compose file$/i), 'services:')
    await user.click(screen.getByRole('button', { name: /import template/i }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/v1/templates/custom')
    expect(JSON.parse(init.body as string)).toMatchObject({
      id: 'my-app',
      name: 'My App',
    })
  })

  it('blocks a lower-case variable name', async () => {
    const user = userEvent.setup()
    const fetchMock = setup()
    await user.click(screen.getByRole('button', { name: /import compose/i }))
    await user.type(screen.getByLabelText(/^name$/i), 'App')
    await user.type(screen.getByLabelText(/^compose file$/i), 'services:')
    await user.type(screen.getByLabelText(/variables/i), 'db_pass')
    await user.click(screen.getByRole('button', { name: /import template/i }))
    expect(await screen.findByText(/UPPER_CASE/)).toBeTruthy()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
