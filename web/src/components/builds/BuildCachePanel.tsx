import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BroomGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { MetricTile, SkeletonList } from '@/components/kit'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useBuildCache } from '../../queries/buildCenter'
import { BuildCachePruneDialog } from './BuildCachePruneDialog'
import { sizeLabel } from './buildFormat'

interface Props {
  pruneOpen: boolean
  onPruneOpenChange: (open: boolean) => void
}

export function BuildCachePanel({ pruneOpen, onPruneOpenChange }: Props) {
  const { t } = useTranslation('buildCenter')
  const [node, setNode] = useState('')
  const isRoot = useIsRoot()
  const cache = useBuildCache(node)
  const data = cache.data

  return (
    <section
      className="flex flex-col gap-3"
      aria-labelledby="build-cache-title"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="build-cache-title" className="text-base font-semibold">
          {t('cache.title')}
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          {data && data.nodes.length > 1 ? (
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              {t('node.label')}
              <select
                className="h-8 rounded-md border border-input bg-background px-2 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                value={node}
                onChange={(e) => {
                  setNode(e.target.value)
                }}
              >
                {data.nodes.map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.id === '' ? t('node.local') : n.name}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          {isRoot && data?.supported ? (
            <Button
              disabled={data.reclaimable_bytes === 0}
              onClick={() => {
                onPruneOpenChange(true)
              }}
            >
              <BroomGlassIcon size="1.4em" aria-hidden="true" />
              {t('cache.prune')}
            </Button>
          ) : null}
        </div>
      </div>

      {cache.isPending ? <SkeletonList rows={2} /> : null}

      {cache.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t('cache.errorTitle')}</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center gap-3">
            {cache.error.message}
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void cache.refetch()
              }}
            >
              {t('retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      {data && !data.supported ? (
        <Alert>
          <AlertTitle>{t('cache.unsupportedTitle')}</AlertTitle>
          <AlertDescription>
            {data.reason ?? t('cache.unsupported')}
          </AlertDescription>
        </Alert>
      ) : null}

      {data?.supported ? (
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <MetricTile
            label={t('cache.total')}
            value={sizeLabel(data.total_bytes)}
          />
          <MetricTile
            label={t('cache.inUse')}
            value={sizeLabel(data.in_use_bytes)}
            tone="info"
          />
          <MetricTile
            label={t('cache.reclaimable')}
            value={sizeLabel(data.reclaimable_bytes)}
            tone={data.reclaimable_bytes > 0 ? 'warning' : 'neutral'}
          />
          <MetricTile label={t('cache.records')} value={data.record_count} />
        </div>
      ) : null}

      <BuildCachePruneDialog
        node={node}
        open={pruneOpen}
        onOpenChange={onPruneOpenChange}
      />
    </section>
  )
}
