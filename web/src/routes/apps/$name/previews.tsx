import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { PreviewEnvironmentsCard } from '../../../components/PreviewEnvironmentsCard'
import { PreviewPolicyCard } from '../../../components/PreviewPolicyCard'
import { PreviewLifecycleCard } from '../../../components/PreviewLifecycleCard'
import { HelpLink } from '../../../components/HelpLink'

export const Route = createFileRoute('/apps/$name/previews')({
  component: PreviewsSection,
})

function PreviewsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <div className="flex justify-end">
        <HelpLink path="/previews" label="Preview environments guide" />
      </div>
      <PreviewEnvironmentsCard app={app} />
      <PreviewPolicyCard appName={name} />
      <PreviewLifecycleCard appName={name} />
    </div>
  )
}
