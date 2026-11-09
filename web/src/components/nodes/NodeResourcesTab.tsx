import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatBytes } from '../../lib/format'
import {
  useNodeResources,
  type NodeResource,
} from '../../queries/nodeResources'

function ResourceName({ r }: { r: NodeResource }) {
  return r.kind === 'app' ? (
    <Link
      to="/apps/$name"
      params={{ name: r.name }}
      className="font-medium text-foreground hover:underline"
    >
      {r.name}
    </Link>
  ) : (
    <Link
      to="/databases/$name"
      params={{ name: r.name }}
      className="font-medium text-foreground hover:underline"
    >
      {r.name}
    </Link>
  )
}

function ResourceRows({ rows }: { rows: NodeResource[] }) {
  const { t } = useTranslation('nodes')
  return (
    <ul className="divide-y divide-border" aria-label={t('resourcesTab.list')}>
      {rows.map((r) => (
        <li
          key={`${r.kind}:${r.name}`}
          className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-sm"
        >
          <Badge variant="outline">{t(`resourcesTab.kind.${r.kind}`)}</Badge>
          <ResourceName r={r} />
          <span className="min-w-0 truncate text-muted-foreground">
            {r.detail}
          </span>
          <span className="ml-auto flex items-center gap-3 text-xs text-muted-foreground tabular-nums">
            <span>
              {r.memory_bytes
                ? t('resourcesTab.memory', {
                    value: formatBytes(r.memory_bytes),
                  })
                : t('resourcesTab.noLimit')}
            </span>
            {r.nano_cpus ? (
              <span>
                {t('resourcesTab.cpu', {
                  value: (r.nano_cpus / 1e9).toFixed(2),
                })}
              </span>
            ) : null}
            {r.replicas && r.replicas > 1 ? (
              <span>{t('resourcesTab.replicas', { count: r.replicas })}</span>
            ) : null}
            <Badge variant={r.status === 'active' ? 'success' : 'muted'}>
              {t(`resourcesTab.status.${r.status}`)}
            </Badge>
          </span>
        </li>
      ))}
    </ul>
  )
}

/** NodeResourcesTab lists what is placed on a node and what a drain would move. */
export function NodeResourcesTab({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const query = useNodeResources(nodeId)
  const rows = query.data?.resources ?? []
  const dbs = rows.filter((r) => r.kind === 'database')
  const apps = rows.filter((r) => r.kind === 'app')

  let body: ReactNode
  if (query.isPending) {
    body = (
      <div className="space-y-2" aria-hidden="true">
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-2/3" />
      </div>
    )
  } else if (query.isError) {
    body = (
      <Alert variant="destructive">
        <AlertDescription>{query.error.message}</AlertDescription>
      </Alert>
    )
  } else {
    body = (
      <Tabs defaultValue="placed">
        <TabsList>
          <TabsTrigger value="placed">
            {t('resourcesTab.placedTab', { count: rows.length })}
          </TabsTrigger>
          <TabsTrigger value="drain">{t('resourcesTab.drainTab')}</TabsTrigger>
        </TabsList>
        <TabsContent value="placed" className="pt-2">
          {rows.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('resourcesTab.empty')}
            </p>
          ) : (
            <ResourceRows rows={rows} />
          )}
        </TabsContent>
        <TabsContent value="drain" className="space-y-2 pt-2">
          <p className="text-sm text-foreground" role="status">
            {rows.length === 0
              ? t('resourcesTab.drainNothing')
              : t('resourcesTab.drainSummary', {
                  apps: apps.length,
                  databases: dbs.length,
                })}
          </p>
          {rows.length > 0 ? (
            <>
              <p className="text-xs text-muted-foreground">
                {t('resourcesTab.drainOrder')}
              </p>
              <ResourceRows rows={[...dbs, ...apps]} />
            </>
          ) : null}
        </TabsContent>
      </Tabs>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('resourcesTab.title')}</CardTitle>
      </CardHeader>
      <CardContent aria-live="polite" aria-busy={query.isPending}>
        {body}
      </CardContent>
    </Card>
  )
}

export default NodeResourcesTab
