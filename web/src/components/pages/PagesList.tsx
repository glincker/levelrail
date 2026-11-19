import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LayoutTemplateGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
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
import { deployAttemptsQueryOptions } from '../../queries/deployAttempts'
import { usePagesSites, type PagesSite } from '../../queries/pagesSites'
import { NewSiteDialog } from './NewSiteDialog'

function SiteRow({
  site,
  app,
}: Readonly<{ site: PagesSite; app: AppListEntry | undefined }>) {
  const { t } = useTranslation('pagesSites')
  // One cheap read per row; the newest attempt is the last deploy.
  const attempts = useQuery({
    ...deployAttemptsQueryOptions(site.name),
    retry: false,
  })
  const lastAt =
    site.last_deploy_at ??
    attempts.data?.[0]?.finished_at ??
    attempts.data?.[0]?.started_at
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
        {site.size_bytes === undefined
          ? t('list.unknown')
          : formatSize(site.size_bytes)}
      </TableCell>
    </TableRow>
  )
}

export function PagesList() {
  const { t } = useTranslation('pagesSites')
  const [creating, setCreating] = useState(false)
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
        <Button size="sm" onClick={() => setCreating(true)}>
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
        <EmptyState
          icon={<LayoutTemplateGlassIcon size="1.4em" />}
          title={t('empty.title')}
          description={t('empty.body')}
          action={
            <Button size="sm" onClick={() => setCreating(true)}>
              {t('newSite')}
            </Button>
          }
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
        open={creating}
        onOpenChange={setCreating}
        existingNames={names}
      />
    </PageTemplate>
  )
}
