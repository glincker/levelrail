import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import templates from '../../locales/en/templates.json'
import { TemplateDeployPrefill } from './TemplateDeployPrefill'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

async function renderPrefill(status: number, body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify(body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    ),
  )
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { templates } } })
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={i18n}>
        <TemplateDeployPrefill slug="ghost" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  navigate.mockReset()
})

describe('TemplateDeployPrefill', () => {
  it('opens the template deploy page for a known slug', async () => {
    await renderPrefill(200, { id: 'ghost', name: 'Ghost', compose: '' })
    await vi.waitFor(() => {
      expect(navigate).toHaveBeenCalledWith(
        expect.objectContaining({ params: { id: 'ghost' } }),
      )
    })
  })

  it('shows a not-found state for an unknown slug', async () => {
    await renderPrefill(404, { error: 'service template not found' })
    expect(await screen.findByText('Template not found')).toBeTruthy()
    expect(navigate).not.toHaveBeenCalled()
  })
})
