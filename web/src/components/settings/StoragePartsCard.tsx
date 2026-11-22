import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { WrenchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '@/lib/format'
import { useCleanupOverview, useRunCleanup } from '../../queries/cleanup'
import { useApplyControlPlaneFix } from '../../queries/controlPlaneHealth'
import { dbSpaceKey, useDBSpace, type DBSpacePart } from '../../queries/dbSpace'

// Sizes the control plane database by kind of history and prunes one kind at a time through
// the cleanup center, so each prune keeps its retention window, audit row and run history.
export function StoragePartsCard() {
  const { t } = useTranslation('storageParts')
  const qc = useQueryClient()
  const space = useDBSpace()
  const overview = useCleanupOverview()
  const run = useRunCleanup()
  const fix = useApplyControlPlaneFix()
  const partLabel = (id: string) => {
    switch (id) {
      case 'deploys':
        return t('parts.deploys')
      case 'events':
        return t('parts.events')
      case 'audit':
        return t('parts.audit')
      default:
        return t('parts.metrics')
    }
  }
  const parts = space.data?.parts ?? []
  const busy = run.isPending || fix.isPending

  const done = (part: DBSpacePart, rows: number) => {
    void qc.invalidateQueries({ queryKey: dbSpaceKey })
    toast.add({
      title:
        part.fix && !part.cleanup_targets?.length
          ? t('prunedMetrics')
          : t('pruned', { rows, part: partLabel(part.id) }),
      type: 'success',
    })
  }
  const failed = (e: Error) =>
    toast.add({
      title: t('pruneFailed'),
      description: e.message,
      type: 'error',
    })

  const prune = async (part: DBSpacePart) => {
    try {
      if (!part.cleanup_targets?.length && part.fix) {
        await fix.mutateAsync({ fix: part.fix as 'telemetry_prune' })
        done(part, 0)
        return
      }
      let rows = 0
      for (const id of part.cleanup_targets ?? []) {
        const target = overview.data?.targets.find((x) => x.id === id)
        if (!target) continue
        const res = await run.mutateAsync({ id, days: target.days })
        rows += res.rows
      }
      done(part, rows)
    } catch (e) {
      failed(e instanceof Error ? e : new Error(String(e)))
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent className="space-y-2">
        {parts.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('empty')}</p>
        ) : (
          parts.map((p) => (
            <div
              key={p.id}
              className="flex flex-wrap items-center justify-between gap-3 text-sm"
            >
              <span className="min-w-0 font-medium">{partLabel(p.id)}</span>
              <span className="text-xs text-muted-foreground tabular-nums">
                {formatBytes(p.bytes)}
              </span>
              <Button
                type="button"
                size="sm"
                disabled={busy || (!p.fix && !p.cleanup_targets?.length)}
                onClick={() => {
                  void prune(p)
                }}
              >
                <WrenchGlassIcon size="1.4em" aria-hidden="true" />
                {busy ? t('pruning') : t('prune')}
              </Button>
            </div>
          ))
        )}
        <p className="text-xs text-muted-foreground">{t('cli')}</p>
      </CardContent>
    </Card>
  )
}
