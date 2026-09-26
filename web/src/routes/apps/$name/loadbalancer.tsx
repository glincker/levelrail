import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { LoadBalancerPage } from '../../../components/loadbalancer/LoadBalancerPage'
import { PageSpinner } from '@/components/ui/page-spinner'

export const Route = createFileRoute('/apps/$name/loadbalancer')({
  component: LoadBalancerSection,
  pendingComponent: PageSpinner,
})

function LoadBalancerSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const navigate = useNavigate()

  return (
    <LoadBalancerPage
      appName={name}
      replicas={app.replicas}
      onSetReplicas={() =>
        void navigate({ to: '/apps/$name/deploy-settings', params: { name } })
      }
    />
  )
}
