import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useNodePatchRuns } from '../../queries/nodePatch'
import { PatchRunDetails, PatchRunStatusBadge } from './NodePatchRunDetails'

// Recent patch runs of one node, newest first, each expandable to its details.
export default function NodePatchRuns({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation('nodes')
  const runs = useNodePatchRuns(nodeId)
  const [open, setOpen] = useState<string | null>(null)

  if (runs.isPending) return <Skeleton className="h-10 w-full" />
  if (runs.isError) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span>{t('patch.historyFailed')}</span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => void runs.refetch()}
        >
          {t('patch.retry')}
        </Button>
      </div>
    )
  }
  if (runs.data.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">{t('patch.historyEmpty')}</p>
    )
  }
  return (
    <div className="space-y-1">
      <h3 className="text-xs font-medium text-muted-foreground">
        {t('patch.history')}
      </h3>
      <ul className="divide-y divide-border rounded-md border border-border">
        {runs.data.slice(0, 10).map((run) => {
          const expanded = open === run.id
          return (
            <li key={run.id} className="px-3 py-2">
              <button
                type="button"
                aria-expanded={expanded}
                onClick={() => setOpen(expanded ? null : run.id)}
                className="flex w-full flex-wrap items-center gap-x-3 gap-y-1 text-left text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="text-foreground">
                  {new Date(run.started_at).toLocaleString()}
                </span>
                <span className="text-muted-foreground">
                  {t('patch.by', { actor: run.actor })}
                </span>
                <PatchRunStatusBadge status={run.status} />
                <span className="text-muted-foreground">
                  {t('patch.packagesCount', { count: run.packages.length })}
                </span>
              </button>
              {expanded ? (
                <div className="pt-2">
                  <PatchRunDetails run={run} />
                </div>
              ) : null}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
