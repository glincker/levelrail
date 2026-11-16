import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ServerGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Badge } from '@/components/ui/badge'
import { CardGrid, ObjectCardSkeleton } from '@/components/cards/ObjectCard'
import { useBuildRunners } from '../../queries/buildRunners'
import { RunnerCard } from './RunnerCard'

// Where builds can run and how full each runner is; the control plane is always listed.
export function RunnersPanel() {
  const { t } = useTranslation('deploys')
  const { data, isPending, isError, refetch } = useBuildRunners()
  const runners = data?.runners ?? []
  const strategy = data?.strategy
  const strategyLabel = (v: string) =>
    v === 'first'
      ? t('runners.strategies.first')
      : v === 'least_loaded'
        ? t('runners.strategies.least_loaded')
        : v === 'sticky_per_app'
          ? t('runners.strategies.sticky_per_app')
          : v

  return (
    <section aria-labelledby="runners-title" className="space-y-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2
          id="runners-title"
          className="text-sm font-semibold text-foreground"
        >
          {t('runners.title')}
        </h2>
        {strategy ? (
          <Badge variant="outline">
            {t('runners.strategy', {
              strategy: strategyLabel(strategy),
            })}
          </Badge>
        ) : null}
        <Link
          to="/settings/security-runtime"
          search={{ category: 'deploys' }}
          className="text-xs text-muted-foreground hover:text-foreground hover:underline"
        >
          {t('runners.changeStrategy')}
        </Link>
      </div>
      {isPending ? (
        <ObjectCardSkeleton count={2} footer={false} />
      ) : isError ? (
        <EmptyState
          kind="error"
          icon={<ServerGlassIcon className="size-5" />}
          title={t('runners.error.title')}
          description={t('runners.error.description')}
          action={
            <Button variant="outline" size="sm" onClick={() => void refetch()}>
              {t('runners.error.retry')}
            </Button>
          }
        />
      ) : (
        <>
          {runners.length > 0 ? (
            <CardGrid min="md">
              {runners.map((r) => (
                <li key={r.id} className="flex">
                  <RunnerCard runner={r} />
                </li>
              ))}
            </CardGrid>
          ) : null}
          {runners.every((r) => r.kind === 'control_plane') ? (
            <EmptyState
              kind="first-run"
              icon={<ServerGlassIcon className="size-5" />}
              title={t('runners.empty.title')}
              description={t('runners.empty.description')}
              helpPath="/build-node-routing"
              helpLabel={t('runners.empty.docs')}
              action={
                <Button
                  size="sm"
                  nativeButton={false}
                  render={<Link to="/nodes" />}
                >
                  {t('runners.empty.action')}
                </Button>
              }
            />
          ) : null}
        </>
      )}
    </section>
  )
}
