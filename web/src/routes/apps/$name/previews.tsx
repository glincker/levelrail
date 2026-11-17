import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useApp } from '../../../queries/apps'
import { useGitSource } from '../../../queries/gitSources'
import { usePreviewEnvironments } from '../../../queries/previewEnvironments'
import { showPreviewExtras } from '../../../lib/appTabState'
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
  const previews = usePreviewEnvironments(name)
  const extras = showPreviewExtras({
    loading: source.isLoading || previews.isLoading,
    prPreviewsOn: source.data?.preview_enabled ?? false,
    previewCount: previews.data?.length ?? 0,
  })

  return (
    <div className="space-y-6">
      {prPreviews ? (
        <PreviewEnvironmentsCard app={app} />
      ) : (
        <p className="text-sm text-muted-foreground" role="status">
          {t('pushOnlyProvider')}
        </p>
      )}
      {extras ? <PreviewTtlCard /> : null}
      <BranchDeploysCard appName={name} />
      {extras ? <DeployContextsCard appName={name} /> : null}
      {extras && prPreviews ? <PreviewPolicyCard appName={name} /> : null}
      {extras && prPreviews ? <PreviewLifecycleCard appName={name} /> : null}
    </div>
  )
}
