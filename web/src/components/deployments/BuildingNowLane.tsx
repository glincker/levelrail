import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Progress } from '@/components/ui/progress'
import type { Deployment } from '../../types/deployment'
import { headline, stepPercent } from '../../lib/deploymentPresentation'
import { StatusCell } from './DeploymentRow'

export interface BuildingNowLaneProps {
  rows: Deployment[]
  now: number
  cancelSupported: boolean
  onOpen: (id: string) => void
  onCancel: (d: Deployment) => void
}

export function BuildingNowLane({
  rows,
  now,
  cancelSupported,
  onOpen,
  onCancel,
}: BuildingNowLaneProps) {
  return (
    <section
      aria-label="Building now"
      className={cn('flex flex-col gap-2', rows.length === 0 && 'sr-only')}
    >
      <h2 className="text-sm font-medium">Building now</h2>
      <div aria-live="polite" aria-atomic="false">
        <p className="sr-only">
          {rows.length === 0
            ? 'No deployments in progress'
            : `${String(rows.length)} deployments in progress`}
        </p>
        {rows.length > 0 && (
          <ul className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
            {rows.map((d) => {
              const pct = stepPercent(d)
              return (
                <li
                  key={d.id}
                  className="flex flex-col gap-2 rounded-xl border border-border bg-card p-3 shadow-raised"
                >
                  <button
                    type="button"
                    onClick={() => {
                      onOpen(d.id)
                    }}
                    className="flex min-w-0 flex-col gap-1.5 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
                  >
                    <span className="flex items-center justify-between gap-2">
                      <span className="truncate text-sm font-medium">
                        {d.app}
                      </span>
                      <StatusCell d={d} now={now} />
                    </span>
                    <span
                      className="truncate text-xs text-muted-foreground"
                      title={headline(d)}
                    >
                      {headline(d)}
                    </span>
                    {d.status === 'building' && pct !== null && (
                      <Progress value={pct} aria-label="Step progress" />
                    )}
                  </button>
                  {d.status === 'queued' || d.status === 'building' ? (
                    <div className="flex justify-end">
                      <Button
                        variant="outline"
                        size="xs"
                        disabled={!cancelSupported}
                        title={
                          cancelSupported
                            ? undefined
                            : 'This server does not support cancelling deploys yet'
                        }
                        onClick={() => {
                          onCancel(d)
                        }}
                      >
                        Cancel
                      </Button>
                    </div>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </section>
  )
}
