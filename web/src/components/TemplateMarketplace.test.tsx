import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import templates from '../locales/en/templates.json'
import type { ServiceTemplateListItem } from '../queries/serviceTemplates'
import { DENSITY_STORAGE_KEY } from '../hooks/useTemplateDensity'
import { TemplateMarketplace } from './TemplateMarketplace'
import { TemplateMonogram } from './TemplateMonogram'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, vars?: Record<string, string | number>) => {
      const found = key
        .split('.')
        .reduce<unknown>(
          (node, part) => (node as Record<string, unknown> | undefined)?.[part],
          templates,
        )
      const text = typeof found === 'string' ? found : key
      return text.replace(/\{\{(\w+)\}\}/g, (_m, name: string) =>
        String(vars?.[name] ?? ''),
      )
    },
  }),
}))

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

// jsdom has no layout, so the real virtualizer would render no rows.
vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 100,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 100,
      })),
    measureElement: () => undefined,
    scrollToOffset: () => undefined,
  }),
}))

vi.mock('./TemplateLogoTile', () => ({
  TemplateLogoTile: ({ name }: { name: string }) => <span>{name[0]}</span>,
}))

const CATALOG: ServiceTemplateListItem[] = [
  {
    id: 'node-postgres-starter',
    name: 'Node.js API + PostgreSQL',
    slogan: 'A minimal Node service wired to Postgres',
    category: 'Starter Kits',
    documentation_url: '',
    requires_configuration: false,
    recommended_memory_bytes: 256 * 1024 * 1024,
  },
  {
    id: 'grafana',
    name: 'Grafana',
    slogan: 'Dashboards for metrics',
    category: 'Monitoring',
    documentation_url: '',
    requires_configuration: false,
  },
]

vi.mock('../queries/serviceTemplates', () => ({
  useServiceTemplates: () => ({
    isLoading: false,
    isError: false,
    data: CATALOG,
  }),
}))
vi.mock('../queries/customTemplates', () => ({
  useCustomTemplates: () => ({ data: [] }),
}))

function renderPage() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TemplateMarketplace onSelect={() => {}} onDeployNow={() => {}} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('TemplateMarketplace', () => {
  it('shows the starter kit band on the default view', () => {
    renderPage()
    expect(
      screen.getByRole('heading', { name: 'Starter kits' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Node.js, PostgreSQL')).toBeInTheDocument()
  })

  it('hides the starter kit band while searching', () => {
    renderPage()
    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'grafana' },
    })
    expect(
      screen.queryByRole('heading', { name: 'Starter kits' }),
    ).not.toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent('1 of 2 templates')
  })

  it('shows a no-results state with a clear action', () => {
    renderPage()
    fireEvent.change(screen.getByRole('searchbox'), {
      target: { value: 'nothing-here' },
    })
    expect(screen.getByText('No templates match')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(screen.getByRole('status')).toHaveTextContent('2 of 2 templates')
  })

  it('persists the density choice', () => {
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'List' }))
    expect(window.localStorage.getItem(DENSITY_STORAGE_KEY)).toBe('compact')
    expect(screen.getByRole('button', { name: 'List' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })

  it('survives storage being unavailable', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'List' }))
    expect(screen.getByRole('button', { name: 'List' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })
})

describe('TemplateMonogram', () => {
  it('uses the same tint for the same id and the first letter only', () => {
    render(
      <>
        <TemplateMonogram id="kutt" name="Kutt URL" />
        <TemplateMonogram id="kutt" name="Kutt URL" />
      </>,
    )
    const marks = screen.getAllByText('K')
    expect(marks).toHaveLength(2)
    expect(marks[0]?.dataset.tone).toBe(marks[1]?.dataset.tone)
  })
})
