import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TemplateDeployForm } from './TemplateDeployForm'
import templatesEn from '../../locales/en/templates.json'
import type { TemplateManifest } from '../../types/compose'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
  useNavigate: () => navigate,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['templates'],
  defaultNS: 'templates',
  resources: { en: { templates: templatesEn } },
  interpolation: { escapeValue: false },
})

const manifest: TemplateManifest = {
  id: 'wikijs',
  name: 'Wiki.js',
  description: 'A wiki.',
  category: 'Productivity',
  documentation_url: 'https://example.com',
  env: [
    { key: 'DB_PASSWORD', label: 'Database password', generate: 'password' },
    { key: 'ADMIN_EMAIL', label: 'Admin email', required: true },
    { key: 'SITE_TITLE', label: 'Site title', default: 'My wiki' },
  ],
  domains: [{ service: 'wiki', port: 3000 }],
}

function wrap() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <TemplateDeployForm manifest={manifest} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

function stubFetch() {
  const fn = vi.fn(() =>
    Promise.resolve(
      new Response(
        JSON.stringify({
          app_id: 'wiki',
          services: [{ name: 'wiki-app' }],
          revision: 1,
        }),
        { status: 200 },
      ),
    ),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
  navigate.mockClear()
})

describe('TemplateDeployForm states', () => {
  it('shows required first, generated with controls, optional collapsed', () => {
    wrap()
    expect(screen.getByLabelText(/Admin email/)).toBeInTheDocument()
    expect(screen.getByText('Generated for you')).toBeInTheDocument()
    expect(screen.getByLabelText(/Database password/)).toHaveAttribute(
      'type',
      'password',
    )
    expect(screen.getByRole('button', { name: 'Regenerate' })).toBeVisible()
    const toggle = screen.getByRole('button', { name: /Optional settings/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByLabelText('Site title')).not.toBeVisible()
    expect(screen.getAllByRole('button', { name: 'Deploy' })).toHaveLength(1)
  })

  it('shows the default as a placeholder once optional settings open', async () => {
    wrap()
    await userEvent.click(
      screen.getByRole('button', { name: /Optional settings/ }),
    )
    expect(screen.getByLabelText('Site title')).toHaveAttribute(
      'placeholder',
      'My wiki',
    )
  })

  it('reveals a generated secret and regenerates it', async () => {
    wrap()
    const field = screen.getByLabelText(/Database password/)
    await userEvent.click(screen.getByRole('button', { name: 'Reveal' }))
    expect(field).toHaveAttribute('type', 'text')
    const first = (field as HTMLInputElement).value
    expect(first).not.toBe('')
    await userEvent.click(screen.getByRole('button', { name: 'Regenerate' }))
    expect((field as HTMLInputElement).value).not.toBe(first)
  })

  it('blocks deploy inline when a required value is missing', async () => {
    const fetchFn = stubFetch()
    wrap()
    await userEvent.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(
      await screen.findByText('Admin email is required'),
    ).toBeInTheDocument()
    expect(screen.getByLabelText(/Admin email/)).toHaveAttribute(
      'aria-invalid',
      'true',
    )
    expect(fetchFn).not.toHaveBeenCalled()
  })

  it('deploys with only the filled values and opens the app', async () => {
    const fetchFn = stubFetch()
    wrap()
    await userEvent.type(screen.getByLabelText(/Admin email/), 'a@b.co')
    await userEvent.click(screen.getByRole('button', { name: 'Deploy' }))
    await waitFor(() => expect(navigate).toHaveBeenCalled())
    const init = fetchFn.mock.calls[0] as unknown as [string, RequestInit]
    expect(init[0]).toBe('/api/v1/service-templates/wikijs/install')
    expect(JSON.parse(init[1].body as string)).toEqual({
      env: { ADMIN_EMAIL: 'a@b.co' },
      domains: {},
    })
  })
})
