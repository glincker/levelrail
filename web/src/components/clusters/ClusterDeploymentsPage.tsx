import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { BoxesGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { EmptyState } from '@/components/ui/empty-state'
import { isClustersOff, useClusters } from '../../queries/clusters'
import { ClusterDeployments } from './ClusterDeployments'
import { DeployForm } from './DeployForm'

export function ClusterDeploymentsPage({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('clusters')
  const { isError, error } = useClusters()
  const off = isError && isClustersOff(error)

  return (
    <PageTemplate
      icon={<BoxesGlassIcon size="1.4em" />}
      title={name}
      description={t('deploy.pageDescription')}
      width="wide"
      breadcrumb={
        <Link
          to="/clusters"
          className="text-sm text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          {t('detail.back')}
        </Link>
      }
    >
      {off ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<BoxesGlassIcon className="size-6" />}
          title={t('disabled.title')}
          description={t('disabled.description')}
        />
      ) : (
        <div className="flex flex-col gap-4">
          <DeployForm cluster={name} />
          <ClusterDeployments cluster={name} />
        </div>
      )}
    </PageTemplate>
  )
}
