import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import templatesEn from '../locales/en/templates.json'
import { BrowseTemplatesFields } from './BrowseTemplatesFields'

// ServiceTemplateGrid's card links each template's name/logo to
// /templates/$id (outside any dialog), the same reason ModelRow.test.tsx
// stubs Link: useLinkProps throws without a real <RouterProvider>, which
// this suite never mounts since it only exercises the grid/form, not
// navigation.
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      params,
      ...rest
    }: {
      children?: ReactNode
      to?: string
      params?: Record<string, string>
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={`${to?.replace('$id', params?.id ?? '') ?? ''}`} {...rest}>
        {children}
      </a>
    ),
  }
})

function fakeJsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

const testI18n = createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { templates: templatesEn } },
  defaultNS: 'templates',
  interpolation: { escapeValue: false },
})

function renderFields() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <BrowseTemplatesFields open onCreated={() => {}} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('BrowseTemplatesFields', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('shows a RAM advisory badge only for templates that declare one', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn<(input: RequestInfo | URL) => Promise<Response>>(() =>
        Promise.resolve(
          fakeJsonResponse([
            {
              id: 'ollama-mistral',
              name: 'Ollama: Mistral 7B',
              slogan: "Mistral's 7B instruct model.",
              category: 'AI',
              documentation_url: 'https://ollama.com/library/mistral',
              recommended_memory_bytes: 6 * 1024 * 1024 * 1024,
            },
            {
              id: 'n8n',
              name: 'n8n',
              slogan: 'Build automations.',
              category: 'Automation',
              documentation_url: 'https://docs.n8n.io',
            },
          ]),
        ),
      ),
    )

    renderFields()

    expect(
      await screen.findByText('~6.0 GiB RAM recommended'),
    ).toBeInTheDocument()
    expect(await screen.findByText('n8n')).toBeInTheDocument()
    expect(screen.getAllByText(/RAM recommended/)).toHaveLength(1)
  })
})
