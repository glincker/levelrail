import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { PreviewEnvironmentsCard } from '../../../components/PreviewEnvironmentsCard'
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

  return (
    <div className="space-y-6">
      <PreviewEnvironmentsCard app={app} />
      <BranchDeploysCard appName={name} />
      <DeployContextsCard appName={name} />
      <PreviewPolicyCard appName={name} />
      <PreviewLifecycleCard appName={name} />
    </div>
  )
}
