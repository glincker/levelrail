import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { useDeployStatus } from '../../../queries/deploys'
import { AppOverviewHero } from '../../../components/AppOverviewHero'
import { AppOverview } from '../../../components/AppOverview'
import { DeployStrategyEditor } from '../../../components/DeployStrategyEditor'
import { ConditionsPanel } from '../../../components/ConditionsPanel'
import { GitSourceCard } from '../../../components/GitSourceCard'

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
export const Route = createFileRoute('/apps/$name/overview')({
  component: OverviewSection,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)

  return (
    <div className="space-y-6">
      <AppOverviewHero app={app} conditions={conditions} />
      <AppOverview app={app} />
      <GitSourceCard app={app} />
      <DeployStrategyEditor app={app} />
      <ConditionsPanel conditions={conditions} />
    </div>
  )
}
