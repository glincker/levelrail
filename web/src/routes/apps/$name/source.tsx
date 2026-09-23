import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { GitSourceCard } from '../../../components/GitSourceCard'
import { PreviewEnvironmentsCard } from '../../../components/PreviewEnvironmentsCard'
import { WebhookDeliveriesPanel } from '../../../components/WebhookDeliveriesPanel'
import { HelpLink } from '../../../components/HelpLink'

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
      <div className="flex justify-end">
        <HelpLink path="/git-integrations" label="Git integrations guide" />
      </div>
      <GitSourceCard app={app} />
      <WebhookDeliveriesPanel app={app} />
      <PreviewEnvironmentsCard app={app} />
    </div>
  )
}
