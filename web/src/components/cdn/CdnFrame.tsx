import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ZapGlassIcon } from '@thesvg/ui-react'
import { PageBanner } from '../PageBanner'
import { LazyCdnDiagram } from '../banners/LazyPass3Diagrams'
import type { CdnWindow } from '../../queries/cdnCache'

const WINDOWS: CdnWindow[] = ['5m', '1h', '24h']

const NAV = [
  { to: '/cdn', key: 'overview' },
  { to: '/cdn/caches', key: 'caches' },
  { to: '/cdn/redirects', key: 'redirects' },
  { to: '/cdn/routing', key: 'routing' },
] as const

const LINK =
  'inline-flex h-8 items-center rounded-md px-3 text-sm text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60'
const LINK_ACTIVE = 'bg-muted font-medium text-foreground'

/** CdnWindowPicker is the time range control shared by the CDN pages. */
export function CdnWindowPicker({
  value,
  onChange,
}: Readonly<{ value: CdnWindow; onChange: (w: CdnWindow) => void }>) {
  const { t } = useTranslation('cdn')
  return (
    <div
      role="group"
      aria-label={t('window.label')}
      className="flex items-center gap-1"
    >
      {WINDOWS.map((w) => (
        <Button
          key={w}
          type="button"
          size="sm"
          variant={w === value ? 'secondary' : 'ghost'}
          aria-pressed={w === value}
          onClick={() => onChange(w)}
        >
          {t(`window.${w}`)}
        </Button>
      ))}
    </div>
  )
}

/**
 * CdnFrame is the shared page frame of the CDN section: title, sub-nav and
 * an optional right-aligned control. Every CDN page renders inside it, so the
 * gutter and the gap between blocks are set once.
 */
export function CdnFrame({
  title,
  description,
  actions,
  children,
}: Readonly<{
  title: string
  description: string
  actions?: ReactNode
  children: ReactNode
}>) {
  const { t } = useTranslation('cdn')
  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<ZapGlassIcon size="1.2em" />}
        titleKey="cdn:nav.label"
        titleText={title}
        descriptionKey="cdn:nav.label"
        descriptionText={description}
        action={actions}
        illustration={<LazyCdnDiagram size="compact" />}
        size="compact"
      />
      <nav
        aria-label={t('nav.label')}
        className="flex flex-wrap gap-1 border-b border-border pb-2"
      >
        {NAV.map(({ to, key }) => (
          <Link
            key={to}
            to={to}
            activeOptions={{ exact: true }}
            className={LINK}
            activeProps={{ className: LINK_ACTIVE, 'aria-current': 'page' }}
          >
            {t(`nav.${key}`)}
          </Link>
        ))}
      </nav>
      {children}
    </div>
  )
}
