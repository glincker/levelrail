import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'
import { useInView } from '../../hooks/useInView'
import type { AppListEntry } from '../../types/appDetail'
import { StatusDot } from '../AppRow'
import { AppRowActions } from '../AppRowActions'
import { ManagedByStackTag } from '../stacks/ManagedByStackNotice'
import { PreviewFrame } from '../pages/SitePreview'
import { FailureReason } from './AppFailure'
import { AppLogo } from './AppMetaChips'
import { LastDeployCell } from './AppMetricCells'
import { useAppRowMetrics } from './useAppRowMetrics'

const CARD_CLASS =
  'group/card relative flex flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xs transition-shadow focus-within:ring-2 focus-within:ring-ring hover:shadow-md motion-reduce:transition-none'

function PreviewPlaceholder({ app }: Readonly<{ app: AppListEntry }>) {
  const { t } = useTranslation('appRows')
  return (
    <div className="flex size-full flex-col items-center justify-center gap-2 bg-gradient-to-br from-muted/60 to-muted/20 text-center">
      <AppLogo image={app.image} />
      <span className="text-xs text-muted-foreground">
        {t('card.noPreview')}
      </span>
    </div>
  )
}

function Thumb({
  app,
  url,
}: Readonly<{ app: AppListEntry; url: string | null }>) {
  const { ref, seen } = useInView<HTMLDivElement>()
  // Only a healthy site is worth loading; a failing one would show an error page.
  const live = url !== null && app.status.variant === 'success'
  return (
    <div
      ref={ref}
      className="relative h-32 w-full overflow-hidden border-b border-border bg-muted/40"
    >
      {live && seen ? (
        <PreviewFrame key={url} url={url} name={app.name} device="card" />
      ) : (
        <PreviewPlaceholder app={app} />
      )}
    </div>
  )
}

export function AppPreviewCard({
  app,
  isStatic,
  selected,
  onSelect,
}: Readonly<{
  app: AppListEntry
  isStatic: boolean
  selected: boolean
  onSelect: (name: string, checked: boolean) => void
}>) {
  const { t } = useTranslation('appRows')
  const { t: tc } = useTranslation('common')
  const metrics = useAppRowMetrics(app.name)
  const domain = app.domains?.[0]
  const failing = app.status.variant === 'destructive'
  const linkClass =
    "truncate text-sm font-semibold after:absolute after:inset-0 after:content-[''] focus-visible:outline-none"
  return (
    <li className={cn(CARD_CLASS, failing && 'border-destructive/40')}>
      <Thumb app={app} url={domain ? `https://${domain}` : null} />
      <div className="flex flex-col gap-1.5 p-3">
        <div className="flex items-center gap-2">
          <StatusDot status={app.status} />
          {isStatic ? (
            <Link
              to="/pages/$name"
              params={{ name: app.name }}
              className={linkClass}
            >
              {app.name}
            </Link>
          ) : (
            <Link
              to="/apps/$name"
              params={{ name: app.name }}
              className={linkClass}
            >
              {app.name}
            </Link>
          )}
          <span className="relative z-10 ml-auto flex items-center gap-1">
            <Checkbox
              checked={selected}
              aria-label={tc('appsList.selectApp', { name: app.name })}
              onCheckedChange={(checked) => {
                onSelect(app.name, checked)
              }}
            />
            <AppRowActions app={app} />
          </span>
        </div>
        <p className="flex items-center gap-1 truncate font-mono text-xs text-muted-foreground">
          <GlobeGlassIcon className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">{domain ?? t('card.noDomain')}</span>
        </p>
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
          {failing ? (
            <FailureReason
              status={app.status}
              name={app.name}
              isStatic={isStatic}
            />
          ) : (
            <span className="truncate text-xs text-muted-foreground">
              {isStatic
                ? `${app.status.label} · ${t('card.staticSite')}`
                : app.status.label}
            </span>
          )}
        </div>
        <ManagedByStackTag stack={app.managed_by_stack} />
        <p className="flex items-center gap-1 text-xs text-muted-foreground">
          <span>{t('card.lastDeploy')}</span>
          <LastDeployCell metrics={metrics} />
        </p>
      </div>
    </li>
  )
}
