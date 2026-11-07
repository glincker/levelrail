import type { AnchorHTMLAttributes, ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import tagsEn from '../../locales/en/tags.json'
import { TagsManager } from './TagsManager'

const abilities = vi.hoisted(() => ({ write: true }))
vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({ role: 'admin', can: () => abilities.write }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    params,
    ...rest
  }: {
    children?: ReactNode
    params?: { name: string }
  } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={`/apps/${params?.name ?? ''}`} {...rest}>
      {children}
    </a>
  ),
}))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'tags'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, tags: tagsEn } },
  interpolation: { escapeValue: false },
})

const TAGS = [
  {
    id: 't1',
    name: 'env:prod',
    created_at: '2026-01-02T00:00:00Z',
    app_count: 3,
    resource_count: 5,
  },
  { id: 't2', name: 'team:web', app_count: 0, resource_count: 0 },
]

let calls: { url: string; method: string; body: string | undefined }[] = []

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <TagsManager />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  abilities.write = true
  calls = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({ url, method, body: init?.body as string | undefined })
      if (method === 'DELETE')
        return Promise.resolve(new Response(null, { status: 204 }))
      if (method === 'PATCH') {
        return Promise.resolve(Response.json({ id: 't1', name: 'env:live' }))
      }
      if (url.endsWith('/apps'))
        return Promise.resolve(Response.json([{ name: 'web' }]))
      return Promise.resolve(Response.json(TAGS))
    }),
  )
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('TagsManager', () => {
  it('renders a row per tag with app and resource counts', async () => {
    renderPage()
    const row = (await screen.findByText('env:prod')).closest('tr')
    expect(row).not.toBeNull()
    expect(row?.textContent).toContain('3')
    expect(row?.textContent).toContain('5')
    expect(screen.getByText('team:web')).toBeTruthy()
  })

  it('renames a tag with PATCH', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(
      await screen.findByRole('button', { name: 'Rename env:prod' }),
    )
    const input = screen.getByRole('textbox', { name: 'New name for env:prod' })
    await user.clear(input)
    await user.type(input, 'env:live')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(
        calls.some((c) => c.method === 'PATCH' && c.url === '/api/v1/tags/t1'),
      ).toBe(true),
    )
    expect(calls.find((c) => c.method === 'PATCH')?.body).toBe(
      JSON.stringify({ name: 'env:live' }),
    )
  })

  it('asks for confirmation before deleting', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(
      await screen.findByRole('button', { name: 'Delete env:prod' }),
    )
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
    await user.click(await screen.findByRole('button', { name: 'Delete tag' }))
    await waitFor(() =>
      expect(
        calls.some((c) => c.method === 'DELETE' && c.url === '/api/v1/tags/t1'),
      ).toBe(true),
    )
  })

  it('lists apps for a tag and hides mutate controls without write', async () => {
    abilities.write = false
    const user = userEvent.setup()
    renderPage()
    await user.click(
      await screen.findByRole('button', { name: 'Show apps tagged env:prod' }),
    )
    const link = await screen.findByRole('link', { name: 'web' })
    expect(link.getAttribute('href')).toBe('/apps/web')
    expect(screen.queryByRole('button', { name: 'Rename env:prod' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Delete env:prod' })).toBeNull()
  })
})
