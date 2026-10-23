import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import templates from '../locales/en/templates.json'
import { ServiceTemplateGrid } from './ServiceTemplateGrid'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    ...rest
  }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a {...rest}>{children}</a>
  ),
}))
vi.mock('./TemplateLogoTile', () => ({ TemplateLogoTile: () => null }))

const LONG = 'Node.js API + PostgreSQL with a very long starter name'

function renderGrid() {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify([
            {
              id: 'node-pg',
              name: LONG,
              slogan: 'A minimal Node HTTP service.',
              category: 'Starter Kits',
              recommended_memory_bytes: 256 * 1024 * 1024,
            },
          ]),
          { headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    ),
  )
  const i18n = createInstance()
  void i18n.use(initReactI18next).init({
    lng: 'en',
    resources: { en: { templates } },
    defaultNS: 'templates',
  })
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={new QueryClient()}>
        <ServiceTemplateGrid
          onSelect={() => undefined}
          onDeployNow={() => undefined}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ServiceTemplateGrid card', () => {
  it('keeps the full name and RAM wording reachable and wrappable', async () => {
    renderGrid()
    const name = await screen.findByText(LONG)
    expect(name.className).toContain('line-clamp-2')
    expect(name.closest('a')).toHaveAttribute('title', LONG)
    const pill = screen.getByText('~256 MiB RAM recommended')
    expect(pill).toHaveAttribute('title', '~256 MiB RAM recommended')
    expect(pill.className).toContain('whitespace-normal')
    expect(pill.className).toContain('max-w-full')
  })
})
