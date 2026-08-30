import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { useDeployStatus } from '../../../queries/deploys'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../queries/deployAttempts'
import { AppOverviewHero } from '../../../components/AppOverviewHero'
import { AppQuickStats } from '../../../components/AppQuickStats'
import { AppOverview } from '../../../components/AppOverview'
import { DeployInProgressBanner } from '../../../components/DeployInProgressBanner'
import { PortEditor } from '../../../components/PortEditor'
import { DeployStrategyEditor } from '../../../components/DeployStrategyEditor'
import { ConditionsPanel } from '../../../components/ConditionsPanel'
import { GitSourceCard } from '../../../components/GitSourceCard'
import { PreviewEnvironmentsCard } from '../../../components/PreviewEnvironmentsCard'
import { StorageAttachmentCard } from '../../../components/StorageAttachmentCard'
import { LogDrainCard } from '../../../components/LogDrainCard'
import { DatabaseAttachmentCard } from '../../../components/DatabaseAttachmentCard'
import { PageSpinner } from '@/components/ui/page-spinner'

// Former "overview" tab of routes/apps/$name.tsx's Tabs component, now a
// real deep-linkable route. Reads app/conditions from the same query
// cache the parent layout route's loader already primed (queries/apps.ts,
// queries/deploys.ts), no fetch of its own: AppOverviewHero's status
// badge and setup checklist are derived from those same two values, not
// a new request.
//
// AppOverviewHero (name, status, domain, image, node, setup checklist)
// is the redesigned top of the page; AppOverview below it is the
// pre-existing raw field grid (memory/CPU/strategy/replicas/probes,
// plus the Node/Project move dialogs), unchanged, just no longer the
// only thing on the page.
//
// This route's own loader primes deploy-attempts (queries/deployAttempts.ts),
// the same query DeployAttemptsList already uses, so a running latest
// attempt renders DeployInProgressBanner above the hero without a second,
// route-specific poll.
export const Route = createFileRoute('/apps/$name/overview')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
  component: OverviewSection,
  pendingComponent: PageSpinner,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)
  const { data: attempts } = useDeployAttempts(name)
  const latestAttempt = attempts[0]

  return (
    <div className="space-y-6">
      {latestAttempt?.status === 'running' ? (
        <DeployInProgressBanner
          appName={name}
          attempt={latestAttempt}
          conditions={conditions}
        />
      ) : null}
      <AppOverviewHero app={app} conditions={conditions} />
      <AppQuickStats appName={name} />
      <AppOverview app={app} />
      <GitSourceCard app={app} />
      <PreviewEnvironmentsCard app={app} />
      <StorageAttachmentCard app={app} />
      <LogDrainCard app={app} />
      <DatabaseAttachmentCard app={app} />
      <PortEditor app={app} />
      <DeployStrategyEditor app={app} />
      <ConditionsPanel conditions={conditions} />
    </div>
  )
}
