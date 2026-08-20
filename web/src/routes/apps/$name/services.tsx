import { createFileRoute } from '@tanstack/react-router'
import { appGroupQueryOptions } from '../../../queries/appGroup'
import { AppServicesPanel } from '../../../components/AppServicesPanel'
import { DeploySpecForm } from '../../../components/DeploySpecForm'

// The multi-service tab: name's sibling services under the same
// store.App (GET /api/v1/apps/{name}/group) plus a form to trigger a
// manual multi-service deploy (POST /api/v1/apps/{name}/deploy-spec).
// Safe to visit for a single-service app too, see AppServicesPanel's own
// doc comment.
export const Route = createFileRoute('/apps/$name/services')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(appGroupQueryOptions(name)),
  component: ServicesSection,
})

function ServicesSection() {
  const { name } = Route.useParams()

  return (
    <div className="space-y-6">
      <AppServicesPanel appName={name} />
      <DeploySpecForm appName={name} />
    </div>
  )
}
