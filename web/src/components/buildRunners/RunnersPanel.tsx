import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ServerGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Badge } from '@/components/ui/badge'
import { CardGrid, ObjectCardSkeleton } from '@/components/cards/ObjectCard'
import { useBuildRunners } from '../../queries/buildRunners'
import { dismissBanner, isBannerDismissed } from '@/lib/bannerDismissal'
import { RunnerCard } from './RunnerCard'
import { RunnerStrip } from './RunnerStrip'
import { FarmSummaryCard } from './FarmSummaryCard'

const ADD_NODE_HINT_ID = 'deploys.add-build-node'

// Where builds can run and how full each runner is; the control plane is always listed.
// Idle pages get a one-line strip, active pages the full cards.
export function RunnersPanel({
  compact = false,
}: Readonly<{ compact?: boolean }>) {
  const { t } = useTranslation('deploys')
  const { data, isPending, isError, refetch } = useBuildRunners()
  const runners = data?.runners ?? []
  const strategy = data?.strategy
  const [hintGone, setHintGone] = useState(() =>
    isBannerDismissed(ADD_NODE_HINT_ID),
  )
  const onlyControlPlane =
    runners.length > 0 && runners.every((r) => r.kind === 'control_plane')
  const multi = runners.length >= 2
  const strategyLabel = (v: string) =>
    v === 'first'
      ? t('runners.strategies.first')
      : v === 'least_loaded'
        ? t('runners.strategies.least_loaded')
        : v === 'sticky_per_app'
          ? t('runners.strategies.sticky_per_app')
          : v

  const addNodeHint =
    onlyControlPlane && !hintGone ? (
      <span className="inline-flex items-center gap-1 text-xs">
        <Link
          to="/nodes"
          search={{ builds: 1 }}
          className="text-muted-foreground hover:text-foreground hover:underline"
        >
          {t('runners.addNodeLink')}
        </Link>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={t('runners.dismissHint')}
          onClick={() => {
            dismissBanner(ADD_NODE_HINT_ID)
            setHintGone(true)
          }}
        >
          <XGlassIcon size="1.2em" aria-hidden="true" />
        </Button>
      </span>
    ) : null
  const strategyRow = multi ? (
    <>
      {strategy ? (
        <Badge variant="outline">
          {t('runners.strategy', { strategy: strategyLabel(strategy) })}
        </Badge>
      ) : null}
      <Link
        to="/settings/security-runtime"
        search={{ category: 'deploys' }}
        className="text-xs text-muted-foreground hover:text-foreground hover:underline"
      >
        {t('runners.changeStrategy')}
      </Link>
    </>
  ) : null

  if (compact && !isError) {
    if (isPending || runners.length === 0) return null
    return (
      <section
        aria-labelledby="runners-title"
        className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border border-border px-3 py-2"
      >
        <h2
          id="runners-title"
          className="text-xs font-semibold text-muted-foreground"
        >
          {t('runners.title')}
        </h2>
        <ul className="flex min-w-0 flex-1 flex-wrap items-center gap-x-4 gap-y-1">
          {runners.map((r) => (
            <RunnerStrip key={r.id} runner={r} />
          ))}
        </ul>
        {strategyRow}
        {addNodeHint}
      </section>
    )
  }

  return (
    <section aria-labelledby="runners-title" className="space-y-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2
          id="runners-title"
          className="text-sm font-semibold text-foreground"
        >
          {t('runners.title')}
        </h2>
        {strategyRow}
        {addNodeHint}
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
      ) : runners.length > 0 ? (
        <>
          <FarmSummaryCard />
          <CardGrid min="md">
            {runners.map((r) => (
              <li key={r.id} className="flex">
                <RunnerCard runner={r} />
              </li>
            ))}
          </CardGrid>
        </>
      ) : null}
    </section>
  )
}
