import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { HammerIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { MetricTile, SkeletonList } from '@/components/kit'
import { PageTemplate } from '../shell/PageTemplate'
import { DeploymentsSectionTabs } from '../shell/SectionTabGroups'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useBuildCenter } from '../../queries/buildCenter'
import { BuildCachePanel } from './BuildCachePanel'
import { BuildsTable } from './BuildsTable'

export function BuildsPage() {
  const { t } = useTranslation('buildCenter')
  const [pruneOpen, setPruneOpen] = useState(false)
  const isRoot = useIsRoot()
  const builds = useBuildCenter()
  const data = builds.data
  const cacheFull = data?.rows.some((r) => r.hint?.fix === 'prune_cache')

  return (
    <PageTemplate
      title={t('title')}
      description={t('subtitle')}
      width="wide"
      tabs={<DeploymentsSectionTabs />}
    >
      <div className="flex flex-col gap-6">
        {builds.isPending ? <SkeletonList rows={6} /> : null}

        {builds.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t('error.title')}</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center gap-3">
              {builds.error.message}
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void builds.refetch()
                }}
              >
                {t('retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}

        {cacheFull && isRoot ? (
          <Alert variant="destructive">
            <AlertTitle>{t('cacheFull.title')}</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center gap-3">
              {t('cacheFull.description')}
              <Button
                size="sm"
                onClick={() => {
                  setPruneOpen(true)
                }}
              >
                {t('actions.freeCache')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}

        {data ? (
          <>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <MetricTile
                label={t('tiles.queued')}
                value={data.counts.queued}
                tone={data.counts.queued > 0 ? 'warning' : 'neutral'}
              />
              <MetricTile
                label={t('tiles.running')}
                value={data.counts.running}
                tone={data.counts.running > 0 ? 'info' : 'neutral'}
              />
              <MetricTile
                label={t('tiles.finished')}
                value={data.counts.finished}
              />
              <MetricTile
                label={t('tiles.failed')}
                value={data.counts.failed}
                tone={data.counts.failed > 0 ? 'danger' : 'neutral'}
              />
            </div>

            <div className="flex flex-col gap-1 text-xs text-muted-foreground">
              <p>
                {t('limits.perNode', { count: data.max_concurrent_per_node })}
                {data.nodes.length > 0
                  ? ` ${data.nodes
                      .map((n) =>
                        t('limits.node', {
                          node: n.node === '' ? t('node.local') : n.node,
                          running: n.running,
                          waiting: n.waiting,
                        }),
                      )
                      .join(', ')}`
                  : ''}
              </p>
              <p>
                {t('platforms.target', { platform: data.platforms.host })}{' '}
                {data.platforms.multi_platform ? '' : t('platforms.singleOnly')}
              </p>
            </div>

            {data.rows.length === 0 ? (
              <EmptyState
                icon={<HammerIcon className="size-5" />}
                title={t('empty.title')}
                description={t('empty.description')}
              />
            ) : (
              <BuildsTable
                rows={data.rows}
                running={data.counts.running}
                onFixCache={() => {
                  setPruneOpen(true)
                }}
              />
            )}
          </>
        ) : null}

        <BuildCachePanel
          pruneOpen={pruneOpen}
          onPruneOpenChange={setPruneOpen}
        />
      </div>
    </PageTemplate>
  )
}
