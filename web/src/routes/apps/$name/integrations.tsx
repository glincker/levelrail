import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { StorageAttachmentCard } from '../../../components/StorageAttachmentCard'
import { LogDrainCard } from '../../../components/LogDrainCard'
import { DatabaseAttachmentCard } from '../../../components/DatabaseAttachmentCard'

// Former Overview-page cards, split out here since all three attach an
// external resource (bucket, log sink, managed database) to this app.
export const Route = createFileRoute('/apps/$name/integrations')({
  component: IntegrationsSection,
})

function IntegrationsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <StorageAttachmentCard app={app} />
      <LogDrainCard app={app} />
      <DatabaseAttachmentCard app={app} />
    </div>
  )
}
