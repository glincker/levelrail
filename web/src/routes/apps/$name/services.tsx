import { createFileRoute } from '@tanstack/react-router'
import { appGroupQueryOptions } from '../../../queries/appGroup'
import { AppServicesPanel } from '../../../components/AppServicesPanel'
import { DeploySpecForm } from '../../../components/DeploySpecForm'

// Safe to visit for a single-service app too, see AppServicesPanel's own comment.
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
