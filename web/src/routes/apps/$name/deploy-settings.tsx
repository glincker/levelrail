import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { PortEditor } from '../../../components/PortEditor'
import { DeployStrategyEditor } from '../../../components/DeployStrategyEditor'
import { HooksEditor } from '../../../components/HooksEditor'
import { DeploySafetyCard } from '../../../components/DeploySafetyCard'
import { PostDeployWatchCard } from '../../../components/PostDeployWatchCard'
import { BuildCacheCard } from '../../../components/BuildCacheCard'
import { DeployFreezeCard } from '../../../components/DeployFreezeCard'
import { ScheduledDeployCard } from '../../../components/ScheduledDeployCard'
import { CancelSupersededCard } from '../../../components/CancelSupersededCard'
import { PreviewSettingsCard } from '../../../components/PreviewSettingsCard'
import { SupplyChainSettingsCard } from '../../../components/SupplyChainSettingsCard'
import { SecretScanSettingsCard } from '../../../components/SecretScanSettingsCard'

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
      <DeploySafetyCard appName={name} />
      <PostDeployWatchCard appName={name} />
      <HooksEditor app={app} />
      <DeployFreezeCard appName={name} />
      <ScheduledDeployCard appName={name} />
      <CancelSupersededCard appName={name} />
      <BuildCacheCard appName={name} />
      <PreviewSettingsCard appName={name} />
      <SupplyChainSettingsCard appName={name} />
      <SecretScanSettingsCard appName={name} />
    </div>
  )
}
