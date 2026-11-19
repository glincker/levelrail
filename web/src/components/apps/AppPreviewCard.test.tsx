import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
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
vi.mock('../pages/SitePreview', () => ({
  PreviewFrame: ({ url }: { url: string }) => <iframe title="live" src={url} />,
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

describe('AppPreviewCard', () => {
  it('shows the live preview, domain and last deploy for a healthy app', () => {
    renderCard(app())
    expect(screen.getByTitle('live')).toHaveAttribute(
      'src',
      'https://web.example.com',
    )
    expect(screen.getByText('web.example.com')).toBeInTheDocument()
    expect(screen.getByText('3m ago')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'web' })).toHaveAttribute(
      'href',
      '/apps/web',
    )
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
    expect(screen.queryByTitle('live')).toBeNull()
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
})
