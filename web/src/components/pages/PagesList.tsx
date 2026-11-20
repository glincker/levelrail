import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  GlobeGlassIcon,
  LayoutTemplateGlassIcon,
  PlusGlassIcon,
} from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatAge, formatSize } from '../../lib/format'
import type { AppListEntry } from '../../types/appDetail'
import { appListQueryOptions } from '../../queries/apps'
import {
  usePagesSiteDetail,
  usePagesSites,
  type PagesSite,
} from '../../queries/pagesSites'
import { PreviewFrame } from './SitePreview'
import { EntryCards } from './EntryCards'
import { NewSiteDialog } from './NewSiteDialog'

function SiteCard({
  site,
  app,
}: Readonly<{ site: PagesSite; app: AppListEntry | undefined }>) {
  const { t } = useTranslation('pagesSites')
  // The list omits size and deploy time, so each rendered card reads its own detail once.
  const detail = usePagesSiteDetail(site.name, site.size_bytes === undefined)
  const lastAt =
    site.last_deploy_at ??
    detail.data?.latest_deploy?.finished_at ??
    detail.data?.latest_deploy?.started_at ??
    detail.data?.updated_at
  const sizeBytes = site.size_bytes ?? detail.data?.size_bytes
  const domain = site.domains[0]
  const variant = app?.status.variant
  const url = domain ? `https://${domain}` : null
  return (
    <li className="group relative flex w-64 flex-col overflow-hidden rounded-xl border border-border bg-card shadow-xs transition-shadow focus-within:ring-2 focus-within:ring-ring hover:shadow-md motion-reduce:transition-none">
      <div className="relative h-40 w-full overflow-hidden border-b border-border bg-muted/40">
        {url ? (
          <PreviewFrame key={url} url={url} name={site.name} device="card" />
        ) : (
          <div className="flex size-full items-center justify-center">
            <GlobeGlassIcon
              className="size-8 text-muted-foreground/60"
              aria-hidden="true"
            />
          </div>
        )}
      </div>
      <div className="flex flex-col gap-1.5 p-3">
        <div className="flex items-center justify-between gap-2">
          <Link
            to="/pages/$name"
            params={{ name: site.name }}
            className="truncate text-sm font-semibold after:absolute after:inset-0 after:content-[''] focus-visible:outline-none"
          >
            {site.name}
          </Link>
          {variant ? (
            <StatusBadge
              variant={variant}
              label={app?.status.label ?? t('status.unknown')}
            />
          ) : null}
        </div>
        <p className="truncate font-mono text-xs text-muted-foreground">
          {domain ?? t('list.noUrl')}
        </p>
        <p className="text-xs text-muted-foreground tabular-nums">
          {lastAt ? formatAge(lastAt) : t('list.none')}
          {sizeBytes === undefined ? '' : ` · ${formatSize(sizeBytes)}`}
        </p>
      </div>
    </li>
  )
}

function CardSkeleton() {
  return (
    <li
      aria-hidden="true"
      className="w-64 overflow-hidden rounded-xl border border-border bg-card"
    >
      <Skeleton className="h-40 w-full rounded-none" />
      <div className="space-y-2 p-3">
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-3 w-1/2" />
      </div>
    </li>
  )
}

export function PagesList({
  initialOpen = false,
  onClosed,
}: Readonly<{ initialOpen?: boolean; onClosed?: () => void }>) {
  const { t } = useTranslation('pagesSites')
  const [creating, setCreating] = useState(initialOpen)
  const [startMode, setStartMode] = useState<'drop' | 'blank'>('drop')
  const openNew = (m: 'drop' | 'blank') => {
    setStartMode(m)
    setCreating(true)
  }
  const sites = usePagesSites()
  const apps = useQuery({ ...appListQueryOptions(), retry: false })
  const names = sites.data?.map((s) => s.name) ?? []

  return (
    <PageTemplate
      icon={<LayoutTemplateGlassIcon size="1.4em" />}
      title={t('title')}
      width="wide"
      actions={
        <Button size="sm" onClick={() => openNew('drop')}>
          <PlusGlassIcon className="size-4" aria-hidden="true" />
          {t('newSite')}
        </Button>
      }
    >
      {sites.isPending ? (
        <ul
          role="status"
          aria-label={t('list.label')}
          className="flex flex-wrap gap-4"
        >
          <CardSkeleton />
          <CardSkeleton />
          <CardSkeleton />
        </ul>
      ) : sites.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('list.loadFailed')}</AlertDescription>
        </Alert>
      ) : sites.data.length === 0 ? (
        <EntryCards
          onDrop={() => openNew('drop')}
          onBlank={() => openNew('blank')}
        />
      ) : (
        <ul aria-label={t('list.label')} className="flex flex-wrap gap-4">
          {sites.data.map((site) => (
            <SiteCard
              key={site.name}
              site={site}
              app={apps.data?.find((a) => a.name === site.name)}
            />
          ))}
        </ul>
      )}
      <NewSiteDialog
        key={startMode}
        initialMode={startMode}
        open={creating}
        onOpenChange={(o) => {
          setCreating(o)
          if (!o) onClosed?.()
        }}
        existingNames={names}
      />
    </PageTemplate>
  )
}
