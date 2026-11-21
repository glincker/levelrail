import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GlobeGlassIcon } from '@thesvg/ui-react'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'
import { autoHostQueryOptions } from '../../queries/certBudget'
import { previewReasonText } from '../../lib/previewReasons'
import { useInView } from '../../hooks/useInView'
import type { AppListEntry } from '../../types/appDetail'
import { StatusDot } from '../AppRow'
import { AppRowActions } from '../AppRowActions'
import { ManagedByStackTag } from '../stacks/ManagedByStackNotice'
import { PreviewCard } from '../PreviewCard'
import { SkeletonLine } from '../kit'
import { FailureReason } from './AppFailure'
import { AppKindCard } from './AppKindCard'
import { isBackendKind } from '../../lib/appKind'
import { AppLogo } from './AppMetaChips'
import { LastDeployCell } from './AppMetricCells'
import { useAppRowMetrics } from './useAppRowMetrics'

const CARD_CLASS =
  'group/card relative flex flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xs transition-shadow focus-within:ring-2 focus-within:ring-ring hover:shadow-md motion-reduce:transition-none'

function PreviewPlaceholder({
  app,
  state,
}: Readonly<{ app: AppListEntry; state?: string }>) {
  const { t } = useTranslation('appRows')
  const reason =
    state === 'failed' ? previewReasonText(app.preview?.reason) : undefined
  let label = t('card.noPreviewYet')
  if (state === 'failed') label = t('card.previewFailed')
  else if (state === 'queued') label = t('card.previewQueued')
  else if (state === 'rendering') label = t('card.previewRendering')
  return (
    <div
      data-testid="card-preview-placeholder"
      className="flex size-full flex-col items-center justify-center gap-1.5 bg-gradient-to-br from-muted/60 to-muted/20 px-3 text-center"
    >
      <AppLogo image={app.image} />
      <span className="text-xs text-muted-foreground">{label}</span>
      {reason ? (
        <span
          className="max-w-full truncate text-xs text-muted-foreground/80"
          title={reason}
        >
          {reason}
        </span>
      ) : null}
    </div>
  )
}

function Thumb({ app }: Readonly<{ app: AppListEntry }>) {
  const kind = app.app_kind
  if (isBackendKind(kind)) {
    return (
      <div className="relative aspect-video w-full overflow-hidden border-b border-border">
        <AppKindCard app={app} kind={kind} />
      </div>
    )
  }
  return <WebThumb app={app} />
}

function WebThumb({ app }: Readonly<{ app: AppListEntry }>) {
  const { ref, seen } = useInView<HTMLDivElement>()
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  const [loadedUrl, setLoadedUrl] = useState<string | null>(null)
  const imageUrl = app.preview?.image_url
  const card = app.preview?.card
  // The URL carries the capture version, so the browser cache serves it
  // after the first load of each deploy.
  const state = app.preview?.state
  const rendering = state === 'queued' || state === 'rendering'
  const failed = state === 'failed'
  const showImage =
    !rendering && !failed && seen && !!imageUrl && failedUrl !== imageUrl
  const showCard =
    !rendering &&
    !showImage &&
    !!card &&
    (failed || !imageUrl || failedUrl === imageUrl)
  const showPlaceholder = !showImage && !showCard
  return (
    <div
      ref={ref}
      className="relative aspect-video w-full overflow-hidden border-b border-border bg-muted/40"
    >
      {showImage ? (
        <>
          <img
            src={imageUrl}
            alt=""
            width={640}
            height={360}
            loading="lazy"
            decoding="async"
            className="size-full object-cover object-top"
            onLoad={() => setLoadedUrl(imageUrl)}
            onError={() => setFailedUrl(imageUrl)}
          />
          {loadedUrl !== imageUrl ? (
            <div
              className="absolute inset-0"
              aria-hidden="true"
              data-testid="card-thumb-skeleton"
            >
              <SkeletonLine className="h-full rounded-none" />
            </div>
          ) : null}
        </>
      ) : null}
      {showCard ? <PreviewCard appName={app.name} meta={card} /> : null}
      {showPlaceholder ? <PreviewPlaceholder app={app} state={state} /> : null}
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
  const autoHost = useQuery({
    ...autoHostQueryOptions(app.name),
    enabled: !domain,
  })
  const address = domain ?? autoHost.data?.hostname
  const failing = app.status.variant === 'destructive'
  const linkClass =
    "truncate text-sm font-semibold after:absolute after:inset-0 after:content-[''] focus-visible:outline-none"
  return (
    <li className={cn(CARD_CLASS, failing && 'border-destructive/40')}>
      <Thumb app={app} />
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
          <span className="truncate">{address ?? t('card.noDomain')}</span>
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
