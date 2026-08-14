import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { EnvEditor } from '../../../components/EnvEditor'
import { SecretsEditor } from '../../../components/SecretsEditor'

// Former "environment" tab, now a real deep-linkable route. Reads app
// data from the query cache the parent layout route's loader already
// primed.
export const Route = createFileRoute('/apps/$name/environment')({
  component: EnvironmentSection,
})

function EnvironmentSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <EnvEditor app={app} />
      <SecretsEditor appName={app.name} />
    </div>
  )
}
