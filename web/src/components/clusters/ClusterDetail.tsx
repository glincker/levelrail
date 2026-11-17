import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useClusterNamespaces, useClusterNodes } from '../../queries/clusters'

export function ClusterDetail({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('clusters')
  const nodes = useClusterNodes(name, true)
  const namespaces = useClusterNamespaces(name, true)

  return (
    <div className="grid gap-4 border-t border-border bg-muted/30 px-3 py-3 md:grid-cols-2">
      <section aria-label={t('detail.nodes')} className="min-w-0">
        <h3 className="mb-1 text-xs font-semibold text-muted-foreground uppercase">
          {t('detail.nodes')}
        </h3>
        {nodes.isPending ? (
          <Skeleton className="h-5 w-40" />
        ) : nodes.isError ? (
          <p className="text-sm text-destructive">{nodes.error.message}</p>
        ) : nodes.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('detail.noNodes')}</p>
        ) : (
          <ul className="flex flex-col gap-1">
            {nodes.data.map((n) => (
              <li
                key={n.name}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                <span className="min-w-0 truncate font-medium">{n.name}</span>
                <Badge variant={n.ready ? 'success' : 'warning'}>
                  {n.ready ? t('detail.ready') : t('detail.notReady')}
                </Badge>
                {n.roles.length > 0 ? (
                  <span className="text-xs text-muted-foreground">
                    {n.roles.join(', ')}
                  </span>
                ) : null}
                <span className="ml-auto text-xs text-muted-foreground tabular-nums">
                  {n.kubelet_version}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
      <section aria-label={t('detail.namespaces')} className="min-w-0">
        <h3 className="mb-1 text-xs font-semibold text-muted-foreground uppercase">
          {t('detail.namespaces')}
        </h3>
        {namespaces.isPending ? (
          <Skeleton className="h-5 w-40" />
        ) : namespaces.isError ? (
          <p className="text-sm text-destructive">{namespaces.error.message}</p>
        ) : namespaces.data.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('detail.noNamespaces')}
          </p>
        ) : (
          <ul className="flex flex-wrap gap-1.5">
            {namespaces.data.map((ns) => (
              <li key={ns.name}>
                <Badge variant="outline">{ns.name}</Badge>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
