import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { useClusterDeployments } from '../../queries/clusters'
import { DeploymentRow } from './DeploymentRow'

export function ClusterDeployments({ cluster }: Readonly<{ cluster: string }>) {
  const { t } = useTranslation('clusters')
  const list = useClusterDeployments(cluster)

  return (
    <section
      aria-label={t('deployments.title')}
      className="flex flex-col gap-1"
    >
      <h2 className="text-sm font-semibold">{t('deployments.title')}</h2>
      {list.isPending ? (
        <Skeleton className="h-10 w-full" />
      ) : list.isError ? (
        <p className="text-sm text-destructive">{list.error.message}</p>
      ) : list.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('deployments.empty')}
        </p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {list.data.map((d) => (
            <DeploymentRow key={d.app} cluster={cluster} deployment={d} />
          ))}
        </ul>
      )}
    </section>
  )
}
