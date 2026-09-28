import { CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import { AppHealthTimeline } from '../AppHealthTimeline'
import { AppOverview } from '../AppOverview'
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
    <Collapsible className="rounded-xl border border-border">
      <CollapsibleTrigger className="group flex items-center gap-2 px-4 py-3 text-sm font-medium">
        <CaretRightIcon
          className="size-3.5 transition-transform group-data-[panel-open]:rotate-90 motion-reduce:transition-none"
          aria-hidden="true"
        />
        Configuration and diagnostics
      </CollapsibleTrigger>
      <CollapsiblePanel>
        <div className="space-y-6 p-4 pt-0">
          <AppHealthTimeline appName={app.name} />
          <AppOverview app={app} />
          <AppPreflightCard appName={app.name} />
          <ConditionsPanel conditions={conditions} />
        </div>
      </CollapsiblePanel>
    </Collapsible>
  )
}
