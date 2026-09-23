import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { StorageAttachmentCard } from '../../../components/StorageAttachmentCard'
import { LogDrainCard } from '../../../components/LogDrainCard'
import { DatabaseAttachmentCard } from '../../../components/DatabaseAttachmentCard'
import { AppIntegrationsCard } from '../../../components/AppIntegrationsCard'

// Former Overview-page cards (bucket/log-sink/database attachment)
// plus the curated third-party tool catalog (AppIntegrationsCard):
// everything here attaches something to this app, either an external
// resource or a catalog add-on.
export const Route = createFileRoute('/apps/$name/integrations')({
  component: IntegrationsSection,
})

function IntegrationsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <AppIntegrationsCard appName={name} />
      <StorageAttachmentCard app={app} />
      <LogDrainCard app={app} />
      <DatabaseAttachmentCard app={app} />
    </div>
  )
}
