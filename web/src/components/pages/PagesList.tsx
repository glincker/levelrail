import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LayoutTemplateGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { StatusBadge } from '@/components/ui/status-badge'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatAge, formatSize } from '../../lib/format'
import type { AppListEntry } from '../../types/appDetail'
import { appListQueryOptions } from '../../queries/apps'
import {
  usePagesSiteDetail,
  usePagesSites,
  type PagesSite,
} from '../../queries/pagesSites'
import { EntryCards } from './EntryCards'
import { NewSiteDialog } from './NewSiteDialog'

function SiteRow({
  site,
  app,
}: Readonly<{ site: PagesSite; app: AppListEntry | undefined }>) {
  const { t } = useTranslation('pagesSites')
  // The list omits size and deploy time, so each rendered row reads its own detail once.
  const detail = usePagesSiteDetail(site.name, site.size_bytes === undefined)
  const lastAt =
    site.last_deploy_at ??
    detail.data?.latest_deploy?.finished_at ??
    detail.data?.latest_deploy?.started_at ??
    detail.data?.updated_at
  const sizeBytes = site.size_bytes ?? detail.data?.size_bytes
  const domain = site.domains[0]
  const variant = app?.status.variant
  return (
    <TableRow>
      <TableCell className="font-medium">
        <Link
          to="/pages/$name"
          params={{ name: site.name }}
          className="underline-offset-2 hover:underline focus-visible:underline"
        >
          {site.name}
        </Link>
      </TableCell>
      <TableCell className="max-w-64 truncate font-mono text-xs">
        {domain ? (
          <a
            href={`https://${domain}`}
            target="_blank"
            rel="noreferrer"
            className="underline-offset-2 hover:underline focus-visible:underline"
          >
            {domain}
          </a>
        ) : (
          <span className="text-muted-foreground">{t('list.noUrl')}</span>
        )}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {lastAt ? formatAge(lastAt) : t('list.none')}
      </TableCell>
      <TableCell>
        {variant ? (
          <StatusBadge
            variant={variant}
            label={app?.status.label ?? t('status.unknown')}
          />
        ) : (
          <span className="text-muted-foreground">{t('list.unknown')}</span>
        )}
      </TableCell>
      <TableCell className="text-right text-muted-foreground tabular-nums">
        {sizeBytes === undefined ? t('list.unknown') : formatSize(sizeBytes)}
      </TableCell>
    </TableRow>
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
      description={t('description')}
      width="wide"
      actions={
        <Button size="sm" onClick={() => openNew('drop')}>
          <PlusGlassIcon className="size-4" aria-hidden="true" />
          {t('newSite')}
        </Button>
      }
    >
      {sites.isPending ? (
        <TableSkeleton rowCount={3} columnCount={5} />
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
        <div className="rounded-lg border border-border bg-card">
          <Table aria-label={t('list.label')}>
            <TableHeader>
              <TableRow>
                <TableHead>{t('list.name')}</TableHead>
                <TableHead>{t('list.url')}</TableHead>
                <TableHead>{t('list.lastDeploy')}</TableHead>
                <TableHead>{t('list.status')}</TableHead>
                <TableHead className="text-right">{t('list.size')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sites.data.map((site) => (
                <SiteRow
                  key={site.name}
                  site={site}
                  app={apps.data?.find((a) => a.name === site.name)}
                />
              ))}
            </TableBody>
          </Table>
        </div>
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
