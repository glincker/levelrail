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
import { ConditionsPanel } from '../../../components/ConditionsPanel'
import { DiagnosisPanel } from '../../../components/DiagnosisPanel'
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
// only thing on the page. Git source, preview environments, storage/log
// drain/database attachments, port, and deploy strategy moved out to
// their own Source/Deploy settings/Integrations tabs (source.tsx,
// deploy-settings.tsx, integrations.tsx): this page is an at-a-glance
// summary again, not every editor stacked in one scroll.
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
      <DiagnosisPanel
        appName={name}
        conditions={conditions}
        latestAttemptStatus={latestAttempt?.status}
      />
      <AppQuickStats appName={name} />
      <AppOverview app={app} />
      <ConditionsPanel conditions={conditions} />
    </div>
  )
}
