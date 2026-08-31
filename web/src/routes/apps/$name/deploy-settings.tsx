import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { PortEditor } from '../../../components/PortEditor'
import { DeployStrategyEditor } from '../../../components/DeployStrategyEditor'

// Former Overview-page cards, split out here since both control how a
// deploy actually rolls out: the port a new container listens on, and
// the strategy/replica count used to cut traffic to it.
export const Route = createFileRoute('/apps/$name/deploy-settings')({
  component: DeploySettingsSection,
})

function DeploySettingsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <PortEditor app={app} />
      <DeployStrategyEditor app={app} />
    </div>
  )
}
