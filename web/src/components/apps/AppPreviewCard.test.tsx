import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import appRowsEn from '../../locales/en/appRows.json'
import type { AppListEntry } from '../../types/appDetail'
import { AppPreviewCard } from './AppPreviewCard'

vi.mock('../AppRowActions', () => ({ AppRowActions: () => null }))
vi.mock('./AppMetricCells', () => ({
  LastDeployCell: () => <span>3m ago</span>,
}))
vi.mock('./useAppRowMetrics', () => ({ useAppRowMetrics: () => undefined }))
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
      params?: { name: string }
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to?.replace('$name', params?.name ?? '')} {...rest}>
        {children}
      </a>
    ),
  }
})

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { appRows: appRowsEn, common: {} } },
  interpolation: { escapeValue: false },
})

function app(over: Partial<AppListEntry> = {}): AppListEntry {
  return {
    name: 'web',
    image: 'nginx:1.27',
    port: 80,
    domains: ['web.example.com'],
    status: { label: 'Healthy', variant: 'success' },
    ...over,
  } as AppListEntry
}

function renderCard(a: AppListEntry, isStatic = false) {
  return render(
    <I18nextProvider i18n={i18n}>
      <ul>
        <AppPreviewCard
          app={a}
          isStatic={isStatic}
          selected={false}
          onSelect={vi.fn()}
        />
      </ul>
    </I18nextProvider>,
  )
}

const IMG = '/api/v1/apps/web/deployments/d1/preview?v=1700000000'

describe('AppPreviewCard', () => {
  it('shows the cached image lazily, with domain and last deploy, and no iframe', () => {
    const { container } = renderCard(app({ preview: { image_url: IMG } }))
    const img = container.querySelector('img')
    expect(img).toHaveAttribute('src', IMG)
    expect(img).toHaveAttribute('loading', 'lazy')
    expect(img).toHaveAttribute('decoding', 'async')
    expect(img).toHaveAttribute('width')
    expect(container.querySelector('iframe')).toBeNull()
    expect(screen.getByText('web.example.com')).toBeInTheDocument()
    expect(screen.getByText('3m ago')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'web' })).toHaveAttribute(
      'href',
      '/apps/web',
    )
  })

  it('shows a skeleton until the image loads', () => {
    const { container } = renderCard(app({ preview: { image_url: IMG } }))
    expect(screen.getByTestId('card-thumb-skeleton')).toBeInTheDocument()
    fireEvent.load(container.querySelector('img') as HTMLImageElement)
    expect(screen.queryByTestId('card-thumb-skeleton')).toBeNull()
  })

  it('falls back to the text card when the image fails', () => {
    const { container } = renderCard(
      app({ preview: { image_url: IMG, card: { title: 'Hello site' } } }),
    )
    fireEvent.error(container.querySelector('img') as HTMLImageElement)
    expect(screen.getByTestId('preview-card')).toHaveTextContent('Hello site')
    expect(container.querySelector('img')).toBeNull()
  })

  it('renders a text card when only metadata exists', () => {
    renderCard(app({ preview: { card: { title: 'Docs home' } } }))
    expect(screen.getByTestId('preview-card')).toHaveTextContent('Docs home')
  })

  it('uses the placeholder when there is no cached preview, never an iframe', () => {
    const { container } = renderCard(app())
    expect(screen.getByText('No preview')).toBeInTheDocument()
    expect(container.querySelector('iframe')).toBeNull()
  })

  it('shows the failure reason and a Fix link, no preview', () => {
    renderCard(
      app({
        status: {
          label: 'Failing',
          variant: 'destructive',
          message: 'readiness probe failing on port 3000',
        },
      }),
    )
    expect(screen.getByText('No preview')).toBeInTheDocument()
    expect(
      screen.getByText('readiness probe failing on port 3000'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /health checks for web/ }),
    ).toHaveAttribute('href', '/apps/web/health')
  })

  it('links static sites to Pages with static wording', () => {
    renderCard(app(), true)
    expect(screen.getByRole('link', { name: 'web' })).toHaveAttribute(
      'href',
      '/pages/web',
    )
    expect(screen.getByText(/Static site/)).toBeInTheDocument()
  })

  it('falls back to the placeholder without a domain', async () => {
    renderCard(app({ domains: [] }))
    expect(screen.getByText('No preview')).toBeInTheDocument()
    expect(screen.getByText('No domain')).toBeInTheDocument()
    await userEvent.tab()
    expect(screen.getByRole('link', { name: 'web' })).toHaveFocus()
  })
  it.each([
    ['api', 'API'],
    ['worker', 'Worker'],
    ['cron', 'Cron'],
    ['database', 'Database'],
  ] as const)(
    'renders a %s kind card instead of a preview box',
    (kind, badge) => {
      renderCard(
        app({
          app_kind: kind,
          image: 'postgres:16',
          summary: {
            ready: 'true',
            replicas: 2,
            last_deploy_at: '2026-01-01T00:00:00Z',
            next_run_at: '2999-01-01T00:00:00Z',
            last_run_at: '2026-01-01T00:00:00Z',
            last_run_status: 'failed',
          },
        }),
      )
      expect(screen.getByTestId(`kind-card-${kind}`)).toHaveTextContent(badge)
      expect(screen.queryByText('No preview')).toBeNull()
    },
  )

  it('shows readiness, replicas and the readiness path on an api card', () => {
    renderCard(
      app({
        app_kind: 'api',
        summary: { ready: 'true', replicas: 3 },
        health: { readiness: { path: '/healthz' } },
      }),
    )
    expect(screen.getByText('Ready')).toBeInTheDocument()
    expect(screen.getByText('3 replicas')).toBeInTheDocument()
    expect(screen.getByText('/healthz')).toBeInTheDocument()
  })

  it('shows next and last run with the status on a cron card', () => {
    renderCard(
      app({
        app_kind: 'cron',
        summary: {
          next_run_at: '2999-01-01T00:00:00Z',
          last_run_at: '2026-01-01T00:00:00Z',
          last_run_status: 'failed',
        },
      }),
    )
    expect(screen.getByText('Next run')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
  })

  it('keeps the thumbnail path for web apps and the placeholder for unknown kinds', () => {
    renderCard(app({ app_kind: 'web', preview: { image_url: IMG } }))
    expect(screen.queryByTestId('kind-card-api')).toBeNull()
    expect(document.querySelector('img')).toHaveAttribute('src', IMG)
  })
})

describe('AppPreviewCard render state', () => {
  it('shows a skeleton while a render is queued', () => {
    renderCard(app({ preview: { state: 'queued', image_url: '/x.jpg' } }))
    expect(screen.getByTestId('card-render-skeleton')).toBeTruthy()
    expect(document.querySelector('img')).toBeNull()
  })
  it('shows a quiet text note when the render failed', () => {
    renderCard(app({ preview: { state: 'failed', reason: 'timeout' } }))
    expect(screen.getByText('Preview unavailable')).toBeTruthy()
  })
})
