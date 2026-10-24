import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BroomIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { useIsRoot } from '../../hooks/useIsRoot'
import { groupTargets, previewTargets } from '../../lib/cleanup'
import { useCleanupOverview } from '../../queries/cleanup'
import { useRuntimeSettings } from '../../queries/runtimeSettings'
import type { CleanupCategory } from '../../types/cleanup'
import { CleanupHero } from './CleanupHero'
import { CleanupTargetRow } from './CleanupTargetRow'

export function CleanupOverviewTab() {
  const { t } = useTranslation('cleanup')
  const isRoot = useIsRoot()
  const overview = useCleanupOverview()
  const settings = useRuntimeSettings()
  const [category, setCategory] = useState<CleanupCategory | null>(null)
  const [reviewOnly, setReviewOnly] = useState(false)

  if (overview.isPending) {
    return (
      <div className="space-y-4" aria-hidden="true">
        <Skeleton className="h-44 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  if (overview.error) {
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-wrap items-center gap-3">
          {t('loadError', { message: overview.error.message })}
          <Button
            size="sm"
            variant="outline"
            onClick={() => void overview.refetch()}
          >
            {t('retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const targets = overview.data.targets
  if (targets.length === 0) {
    return (
      <EmptyState
        icon={<BroomIcon className="size-6" />}
        title={t('empty.title')}
        description={t('empty.body')}
        action={
          <Button
            size="sm"
            variant="outline"
            onClick={() => void overview.refetch()}
          >
            {t('retry')}
          </Button>
        }
      />
    )
  }

  const allowedFor = (key: string): number[] | undefined =>
    settings.data?.settings.find((s) => s.key === key)?.allowed_values

  const visible = reviewOnly ? previewTargets(targets) : targets
  const groups = groupTargets(
    category === null
      ? visible
      : visible.filter((x) => x.category === category),
  )
  const categoryBytes = (c: CleanupCategory) =>
    groupTargets(targets).find((g) => g.category === c)?.bytes ?? 0

  return (
    <div className="space-y-5">
      <CleanupHero
        targets={targets}
        scheduler={overview.data.scheduler}
        filter={category}
        onFilter={(c) => {
          setCategory(c)
          setReviewOnly(false)
        }}
        onReview={() => {
          setCategory(null)
          setReviewOnly(true)
        }}
      />
      {!isRoot ? (
        <p className="text-sm text-muted-foreground">{t('readOnly')}</p>
      ) : null}
      {reviewOnly ? (
        <div className="flex items-center justify-between gap-2 text-sm">
          <p className="text-muted-foreground">
            {t('hero.previewCount', { count: previewTargets(targets).length })}
          </p>
          <Button
            size="xs"
            variant="ghost"
            onClick={() => {
              setReviewOnly(false)
            }}
          >
            {t('hero.clearFilter')}
          </Button>
        </div>
      ) : null}
      <div className="space-y-5">
        {groups.map((g) => (
          <section
            key={g.category}
            aria-labelledby={`cleanup-group-${g.category}`}
            className="rounded-lg border border-border bg-card"
          >
            <header className="sticky top-0 z-10 flex items-baseline justify-between gap-2 rounded-t-lg border-b border-border bg-card px-3 py-2 sm:px-4">
              <h2
                id={`cleanup-group-${g.category}`}
                className="text-sm font-medium text-foreground"
              >
                {t(`categories.${g.category}`)}
              </h2>
              <p className="text-xs text-muted-foreground">
                {t('group.count', { count: g.targets.length })}
              </p>
            </header>
            <ul>
              {g.targets.map((target) => (
                <CleanupTargetRow
                  key={target.id}
                  target={target}
                  categoryBytes={categoryBytes(g.category)}
                  canEdit={isRoot}
                  allowedDays={allowedFor(target.policy_key)}
                />
              ))}
            </ul>
          </section>
        ))}
      </div>
    </div>
  )
}
