import { Suspense } from 'react'
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
import { AutoRollbackCard } from '../../../components/AutoRollbackCard'
import { PredeploySnapshotCard } from '../../../components/PredeploySnapshotCard'
import { AutoRollbackSLOBurnCard } from '../../../components/AutoRollbackSLOBurnCard'
import { CanaryCard } from '../../../components/CanaryCard'
import { ImageAutoUpdateCard } from '../../../components/ImageAutoUpdateCard'
import { BuildHooksCard } from '../../../components/BuildHooksCard'
import { BadgeSettingsCard } from '../../../components/BadgeSettingsCard'
import { GitHubActionsCard } from '../../../components/GitHubActionsCard'
import { CIDeployCard } from '../../../components/CIDeployCard'
import { SettingsGroup } from '../../../components/deploySettings/SettingsGroup'
import { SettingsSectionNav } from '../../../components/deploySettings/SettingsSectionNav'
import { DeployFlowStrip } from '../../../components/deploySettings/DeployFlowStrip'
import { CardHelp } from '../../../components/deploySettings/CardHelp'
import { autoRollbackQueryOptions } from '../../../queries/autoRollback'
import { autoRollbackSLOBurnQueryOptions } from '../../../queries/autoRollbackSLOBurn'
import { imageAutoUpdateQueryOptions } from '../../../queries/imageAutoUpdate'
import { canaryQueryOptions } from '../../../queries/canary'
import { badgeQueryOptions } from '../../../queries/badge'
import { buildHooksQueryOptions } from '../../../queries/buildHooks'
import { CardSkeleton } from '@/components/ui/card-skeleton'

// The one home for deploy settings, grouped by when they act: Build,
// Run, Safety, Automation. The deploys page only links here. Suspense cards
// that the loader does not prime get a CardSkeleton so only they wait.
export const Route = createFileRoute('/apps/$name/deploy-settings')({
  loader: async ({ context: { queryClient }, params: { name } }) => {
    await Promise.all([
      queryClient.ensureQueryData(autoRollbackQueryOptions(name)),
      queryClient.ensureQueryData(autoRollbackSLOBurnQueryOptions(name)),
      queryClient.ensureQueryData(imageAutoUpdateQueryOptions(name)),
      queryClient.ensureQueryData(canaryQueryOptions(name)),
      queryClient.ensureQueryData(badgeQueryOptions(name)),
      queryClient.ensureQueryData(buildHooksQueryOptions(name)),
    ])
  },
  component: DeploySettingsSection,
})

function DeploySettingsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <DeployFlowStrip />
      <SettingsSectionNav />
      <SettingsGroup id="build">
        <CardHelp card="buildHooks">
          <BuildHooksCard appName={name} />
        </CardHelp>
        <CardHelp card="buildCache">
          <BuildCacheCard appName={name} />
        </CardHelp>
      </SettingsGroup>
      <SettingsGroup id="run">
        <CardHelp card="port" recommended>
          <PortEditor app={app} />
        </CardHelp>
        <CardHelp card="strategy">
          <DeployStrategyEditor app={app} />
        </CardHelp>
        <CardHelp card="hooks">
          <HooksEditor app={app} />
        </CardHelp>
      </SettingsGroup>
      <SettingsGroup id="safety">
        <CardHelp card="safety" recommended>
          <DeploySafetyCard appName={name} />
        </CardHelp>
        <CardHelp card="watch" recommended>
          <PostDeployWatchCard appName={name} />
        </CardHelp>
        <CardHelp card="autoRollback">
          <AutoRollbackCard appName={name} />
        </CardHelp>
        <CardHelp card="predeploySnapshot">
          <PredeploySnapshotCard appName={name} />
        </CardHelp>
        <CardHelp card="sloBurn">
          <AutoRollbackSLOBurnCard appName={name} />
        </CardHelp>
        <CardHelp card="canary">
          <CanaryCard appName={name} />
        </CardHelp>
        <CardHelp card="freeze">
          <DeployFreezeCard appName={name} />
        </CardHelp>
        <CardHelp card="supplyChain">
          <SupplyChainSettingsCard appName={name} />
        </CardHelp>
        <CardHelp card="secretScan">
          <SecretScanSettingsCard appName={name} />
        </CardHelp>
      </SettingsGroup>
      <SettingsGroup id="automation">
        <CardHelp card="scheduled">
          <ScheduledDeployCard appName={name} />
        </CardHelp>
        <CardHelp card="cancelSuperseded">
          <CancelSupersededCard appName={name} />
        </CardHelp>
        <CardHelp card="preview">
          <PreviewSettingsCard appName={name} />
        </CardHelp>
        <CardHelp card="imageAutoUpdate">
          <ImageAutoUpdateCard appName={name} />
        </CardHelp>
        <CardHelp card="badge">
          <BadgeSettingsCard appName={name} />
        </CardHelp>
        <CardHelp card="githubActions">
          <Suspense fallback={<CardSkeleton titleWidth="w-44" rows={2} />}>
            <GitHubActionsCard appName={name} />
          </Suspense>
        </CardHelp>
        <CardHelp card="ciDeploy">
          <Suspense fallback={<CardSkeleton titleWidth="w-36" rows={4} />}>
            <CIDeployCard appName={name} />
          </Suspense>
        </CardHelp>
      </SettingsGroup>
    </div>
  )
}
