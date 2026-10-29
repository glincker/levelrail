import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  GitCompareGlassIcon,
  RocketGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import { useApp } from '../queries/apps'
import { useProtectedEnvironment } from '../queries/environments'
import {
  EMPTY_DEPLOY_FILTERS,
  filterDeployAttempts,
} from '../lib/deployAttemptFilter'
import { DeployAttemptFilterBar } from './DeployAttemptFilters'
import { DeployAttemptRow } from './DeployAttemptRow'
import { ProtectedEnvironmentNotice } from './ProtectedEnvironmentNotice'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'

// DeployAttemptsList renders GET /api/v1/apps/{name}/deploy-attempts'
// real, row-per-attempt history (internal/api/deploy_attempts.go), the
// list this task's own design note exists to unblock: a status, an
// image tag, a timestamp, a link into the already-existing (previously
// orphaned) SSE log viewer route, and, for a past successful attempt, a
// "Rollback to this build" action. Rollback needs no dedicated backend
// endpoint: it is exactly POST /api/v1/apps/{name}/deploys
// (useTriggerDeploy, the same mutation DeployTriggerForm's "existing
// image" tab already calls) given that attempt's own Image, per
// handleTriggerDeploy's own doc comment framing a redeploy to an older
// tag as identical to one to a newer tag.
//
// Newest-first, no pagination: matches ListDeployAttempts' own documented
// scope (a known, deliberately deferred follow-up if a single app's
// history grows very large), and this list is not virtualized for the
// same reason AppRow's own list is expected to stay well under the
// virtualization threshold in realistic usage; revisit both together if
// that assumption stops holding.
//
// conditions is optional and only used for the latest attempt's badge
// (which stage is running or failed, see lib/deployStages.ts): older
// attempts have no reconcile-status signal to draw on.
export function DeployAttemptsList({
  appName,
  attempts,
  conditions = [],
  onDeploy,
}: {
  appName: string
  attempts: DeployAttempt[]
  conditions?: ReconcileCondition[]
  onDeploy?: () => void
}) {
  // Capped at 2: a comparison is always between exactly two points (or
  // one point and "current", the per-row Compare-to-current button
  // below), so a third pick replaces the oldest selection rather than
  // growing an N-way comparison this feature doesn't support. Declared
  // before the empty-state early return below so hook order stays fixed
  // regardless of attempts.length.
  const [selected, setSelected] = useState<string[]>([])
  const [ackProtected, setAckProtected] = useState(false)
  const [filters, setFilters] = useState(EMPTY_DEPLOY_FILTERS)
  const { t } = useTranslation('common')
  const { data: app } = useApp(appName)
  const primaryDomain = app?.domains?.[0]
  const visitUrl = primaryDomain ? `https://${primaryDomain}` : undefined
  const protectedEnv = useProtectedEnvironment(app)

  if (attempts.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Deploy history</CardTitle>
        </CardHeader>
        <CardContent>
          <EmptyState
            icon={<RocketGlassIcon className="size-6" />}
            title="No deploys yet"
            description="Deploy an existing image tag, or build one from a git source."
            action={
              onDeploy ? (
                <Button size="sm" onClick={onDeploy}>
                  <RocketGlassIcon
                    className="size-5"
                    data-icon="inline-start"
                  />
                  {t('deployHistory.deployNow')}
                </Button>
              ) : undefined
            }
          />
        </CardContent>
      </Card>
    )
  }

  const latestAttemptId = attempts[0]?.id
  // The latest attempt is pinned above the filters so it is never hidden by
  // them; the filtered list below shows the rest.
  const latestAttempt = attempts[0]
  const matching = filterDeployAttempts(attempts, filters)
  const visibleAttempts = matching.filter((a) => a.id !== latestAttemptId)

  const toggleSelected = (id: string) => {
    setSelected((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length < 2) return [...prev, id]
      return [...prev.slice(1), id]
    })
  }

  // Ordered oldest-first so "from" always reads as the earlier deploy
  // regardless of the order the two rows were clicked in.
  const compareSelectedSearch = (() => {
    if (selected.length !== 2) return null
    const [a, b] = selected
      .map((id) => attempts.find((x) => x.id === id))
      .filter((x): x is DeployAttempt => x !== undefined)
      .sort(
        (x, y) =>
          new Date(x.started_at).getTime() - new Date(y.started_at).getTime(),
      )
    if (!a || !b) return null
    return { from: a.id, to: b.id }
  })()

  const rowFor = (a: DeployAttempt, pinned: boolean) => (
    <DeployAttemptRow
      key={a.id}
      appName={appName}
      visitUrl={visitUrl}
      attempt={a}
      pinned={pinned}
      isLatestAttempt={a.id === latestAttemptId}
      conditions={conditions}
      isSelected={selected.includes(a.id)}
      onToggleSelected={() => toggleSelected(a.id)}
      protectedEnv={protectedEnv}
      ackProtected={ackProtected}
    />
  )

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 space-y-0">
        <CardTitle>Deploy history</CardTitle>
        {compareSelectedSearch ? (
          <Button
            size="sm"
            variant="outline"
            nativeButton={false}
            render={
              <Link
                to="/apps/$name/deploys/compare"
                params={{ name: appName }}
                search={compareSelectedSearch}
              />
            }
          >
            <GitCompareGlassIcon className="size-5" data-icon="inline-start" />
            Compare selected
          </Button>
        ) : selected.length === 1 ? (
          <p className="text-xs text-muted-foreground">
            Select one more deploy to compare
          </p>
        ) : null}
        {onDeploy ? (
          <Button size="sm" onClick={onDeploy} className="ml-auto">
            <RocketGlassIcon className="size-5" data-icon="inline-start" />
            {t('deployHistory.deployNow')}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        {protectedEnv?.protected ? (
          <ProtectedEnvironmentNotice
            id="deploy-attempts-ack-protected"
            environmentName={protectedEnv.name}
            acknowledged={ackProtected}
            onAcknowledgedChange={setAckProtected}
          />
        ) : null}
        {latestAttempt ? (
          <section aria-label={t('deployHistory.latest')} className="mb-4">
            <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {t('deployHistory.latest')}
            </h3>
            <ul className="overflow-hidden rounded-lg border border-border bg-muted/20">
              {rowFor(latestAttempt, true)}
            </ul>
          </section>
        ) : null}
        {attempts.length > 1 ? (
          <h3 className="mb-1.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t('deployHistory.earlier')}
          </h3>
        ) : null}
        {attempts.length > 1 ? (
          <DeployAttemptFilterBar
            attempts={attempts}
            filters={filters}
            onChange={setFilters}
          />
        ) : null}
        {matching.length === 0 ? (
          <EmptyState
            compact
            icon={<SearchGlassIcon className="size-6" />}
            title={t('deployHistory.filters.noMatch')}
            description={t('deployHistory.filters.noMatchHint')}
            action={
              <Button
                size="sm"
                variant="outline"
                onClick={() => setFilters(EMPTY_DEPLOY_FILTERS)}
              >
                {t('deployHistory.filters.clear')}
              </Button>
            }
          />
        ) : null}
        {visibleAttempts.length > 0 ? (
          <ul className="-mx-4 -mb-4 divide-y divide-border border-t border-border">
            {visibleAttempts.map((a) => rowFor(a, false))}
          </ul>
        ) : null}
      </CardContent>
    </Card>
  )
}
