import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { GitSourceCard } from '../../../components/GitSourceCard'
import { GitPushRemoteCard } from '../../../components/GitPushRemoteCard'
import { GitDeploySettingsCard } from '../../../components/GitDeploySettingsCard'
import { CiGateSettings } from '../../../components/apps/CiGateSettings'
import { WebhookDeliveriesPanel } from '../../../components/WebhookDeliveriesPanel'

// Former Overview-page cards, split out here since a preview environment
// is meaningless without a connected git source: the two belong together
// as one "where deploys come from" concern, distinct from Deploy
// settings (port/strategy) and Integrations (storage/log drain/database).
export const Route = createFileRoute('/apps/$name/source')({
  component: SourceSection,
})

function SourceSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <GitSourceCard app={app} />
      <GitPushRemoteCard appName={name} />
      <GitDeploySettingsCard appName={name} />
      <CiGateSettings appName={name} />
      <WebhookDeliveriesPanel app={app} />
    </div>
  )
}
