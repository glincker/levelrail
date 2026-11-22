import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nextStepsEn from '../locales/en/nextSteps.json'
import { AfterInstallSteps } from './AfterInstallSteps'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['nextSteps'],
  defaultNS: 'nextSteps',
  resources: { en: { nextSteps: nextStepsEn } },
  interpolation: { escapeValue: false },
})

function stubTemplate(body: unknown) {
  vi.stubGlobal('fetch', () =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
}

function renderSteps(domains: string[]) {
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <AfterInstallSteps
          app={{ name: 'blog', domains, template_id: 'ghost' }}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AfterInstallSteps', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the template steps with an admin deep link on the app domain', async () => {
    stubTemplate({
      id: 'ghost',
      next_steps: [
        { key: 'open_admin', path: '/ghost/' },
        { key: 'create_login' },
        { key: 'point_domain' },
      ],
    })
    renderSteps(['blog.example.com'])
    const link = await screen.findByRole('link', {
      name: /Open the admin page/,
    })
    expect(link).toHaveAttribute('href', 'https://blog.example.com/ghost/')
    expect(screen.getByText(/Create your admin login/)).toBeInTheDocument()
  })

  it('renders nothing when the template has no steps', async () => {
    stubTemplate({ id: 'ghost' })
    renderSteps([])
    await new Promise((r) => setTimeout(r, 20))
    expect(screen.queryByText(/After install/)).not.toBeInTheDocument()
  })
})
