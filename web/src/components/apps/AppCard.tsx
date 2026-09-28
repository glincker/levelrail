import { Link } from '@tanstack/react-router'
import { Checkbox } from '@/components/ui/checkbox'
import { StatusDot } from '../AppRow'
import { AppRowActions } from '../AppRowActions'
import type { AppListEntry } from '../../types/appDetail'
import { AppLogo, AppMetaChips } from './AppMetaChips'
import {
  ErrorCell,
  LastDeployCell,
  P95Cell,
  TrafficSpark,
} from './AppMetricCells'
import { AppQuickActions } from './AppQuickActions'
import { useAppRowMetrics } from './useAppRowMetrics'

export function AppCard({
  app,
  selected,
  onSelect,
}: {
  app: AppListEntry
  selected: boolean
  onSelect: (name: string, checked: boolean) => void
}) {
  const metrics = useAppRowMetrics(app.name)
  return (
    <div className="group/row relative flex flex-col gap-3 rounded-2xl border border-border bg-card p-4 transition-colors duration-150 hover:border-primary/40">
      <div className="flex items-start gap-3">
        <AppLogo image={app.image} />
        <div className="min-w-0 flex-1 space-y-1">
          <span className="flex min-w-0 items-center gap-2 text-sm font-medium text-foreground">
            <StatusDot status={app.status} />
            <Link
              to="/apps/$name"
              params={{ name: app.name }}
              className="truncate after:absolute after:inset-0"
            >
              {app.name}
            </Link>
          </span>
          <AppMetaChips app={app} />
        </div>
        <span className="relative z-10 flex items-center gap-1">
          <Checkbox
            checked={selected}
            aria-label={`Select ${app.name}`}
            onCheckedChange={(checked) => {
              onSelect(app.name, checked)
            }}
          />
          <AppRowActions app={app} />
        </span>
      </div>
      <TrafficSpark metrics={metrics} name={app.name} width={220} />
      <div className="flex items-center justify-between gap-3">
        <span className="flex items-center gap-3">
          <P95Cell metrics={metrics} />
          <ErrorCell metrics={metrics} />
        </span>
        <span className="flex items-center gap-2">
          <LastDeployCell metrics={metrics} />
          <AppQuickActions app={app} />
        </span>
      </div>
    </div>
  )
}
