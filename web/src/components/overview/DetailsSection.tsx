import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import { AppHealthTimeline } from '../AppHealthTimeline'
import { AppOverview } from '../AppOverview'
import { PlacementCard } from '../PlacementCard'
import { AppPreflightCard } from '../PreflightPanel'
import { ConditionsPanel } from '../ConditionsPanel'

/** Collapsed by default: the full configuration grid, conditions and preflight. */
export function DetailsSection({
  app,
  conditions,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
}) {
  return (
    <Collapsible>
      <CollapsibleTrigger className="group flex items-center gap-2 py-1 text-sm font-medium">
        <ChevronRightGlassIcon
          className="size-4 transition-transform group-data-[panel-open]:rotate-90 motion-reduce:transition-none"
          aria-hidden="true"
        />
        Configuration and diagnostics
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="space-y-6 pt-4">
          <AppHealthTimeline appName={app.name} />
          <AppOverview app={app} />
          <PlacementCard
            kind="app"
            name={app.name}
            currentNodeId={app.node_id}
            volumeNames={app.volumes?.map((v) => v.name) ?? []}
          />
          <AppPreflightCard appName={app.name} />
          <ConditionsPanel conditions={conditions} />
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}
