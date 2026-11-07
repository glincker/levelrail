import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useApp } from '../../../queries/apps'
import { useGitSource } from '../../../queries/gitSources'
import { supportsPrFeatures } from '../../../lib/gitProviderSupport'
import { PreviewEnvironmentsCard } from '../../../components/PreviewEnvironmentsCard'
import { PreviewTtlCard } from '../../../components/PreviewTtlCard'
import { PreviewPolicyCard } from '../../../components/PreviewPolicyCard'
import { BranchDeploysCard } from '../../../components/BranchDeploysCard'
import { DeployContextsCard } from '../../../components/DeployContextsCard'
import { PreviewLifecycleCard } from '../../../components/PreviewLifecycleCard'

export const Route = createFileRoute('/apps/$name/previews')({
  component: PreviewsSection,
})

function PreviewsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { t } = useTranslation('previews')
  const source = useGitSource(name)
  const prPreviews = supportsPrFeatures(source.data?.connection?.provider)

  return (
    <div className="space-y-6">
      {prPreviews ? (
        <PreviewEnvironmentsCard app={app} />
      ) : (
        <p className="text-sm text-muted-foreground" role="status">
          {t('pushOnlyProvider')}
        </p>
      )}
      <PreviewTtlCard />
      <BranchDeploysCard appName={name} />
      <DeployContextsCard appName={name} />
      {prPreviews ? <PreviewPolicyCard appName={name} /> : null}
      {prPreviews ? <PreviewLifecycleCard appName={name} /> : null}
    </div>
  )
}
