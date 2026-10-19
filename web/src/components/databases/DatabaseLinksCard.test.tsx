import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { jsonResponse, newSurfacesI18n } from '../../test/newSurfacesI18n'
import { DatabaseLinksCard } from './DatabaseLinksCard'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      params,
    }: {
      children: ReactNode
      params: { name: string }
    }) => <a href={`/apps/${params.name}`}>{children}</a>,
  }
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function stubApi(links: unknown[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string) =>
      Promise.resolve(
        input.includes('/links') ? jsonResponse({ links }) : jsonResponse([]),
      ),
    ),
  )
}

describe('DatabaseLinksCard', () => {
  it('lists connected apps with their env var', async () => {
    stubApi([{ app: 'web', env_var: 'DATABASE_URL', field: 'url' }])
    render(
      <NewSurfacesProviders>
        <DatabaseLinksCard database="orders-db" />
      </NewSurfacesProviders>,
    )
    expect(await screen.findByRole('link', { name: 'web' })).toHaveAttribute(
      'href',
      '/apps/web',
    )
    expect(screen.getByText('DATABASE_URL')).toBeInTheDocument()
  })

  it('shows the empty state when nothing is connected', async () => {
    stubApi([])
    render(
      <NewSurfacesProviders>
        <DatabaseLinksCard database="orders-db" />
      </NewSurfacesProviders>,
    )
    expect(
      await screen.findByText(
        newSurfacesI18n.t('links.empty', { ns: 'databases' }),
      ),
    ).toBeInTheDocument()
  })
})
