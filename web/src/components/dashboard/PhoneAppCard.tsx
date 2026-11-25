import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  RestoreGlassIcon,
  RocketGlassIcon,
  RotateCwGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import { useAbilities } from '../../hooks/useAbilities'
import { useRedeployApp } from '../../hooks/useRedeployApp'
import { VARIANT_TONE } from '../../lib/dashboardApps'
import { EMPTY_FILTERS } from '../../lib/deploymentFilters'
import { canRollbackTo } from '../../lib/deploymentPresentation'
import { formatRelativeTime } from '../../lib/relativeTime'
import { useRestartApp } from '../../queries/apps'
import { isPendingApproval } from '../../queries/deploys'
import {
  fetchDeploymentPage,
  useDeploymentMutations,
} from '../../queries/deployments'
import type { AppListEntry } from '../../types/appDetail'

type Action = 'restart' | 'redeploy' | 'rollback'

const ACTION_BTN = 'min-h-11 flex-1 gap-1.5'

/** One compact status card per app for phone widths, with confirmed actions. */
export function PhoneAppCard({ app }: { app: AppListEntry }) {
  const { t } = useTranslation('dashboard')
  const { can } = useAbilities()
  const [asking, setAsking] = useState<Action | null>(null)
  const restart = useRestartApp()
  const { redeploy, isPending: redeploying } = useRedeployApp(
    app.name,
    app.image,
    { onSuccess: () => setAsking(null) },
  )
  const { rollback } = useDeploymentMutations()
  // Only fetched once the rollback confirm is open, to find the release to return to.
  const target = useQuery({
    queryKey: ['phone-rollback-target', app.name],
    queryFn: async () => {
      const page = await fetchDeploymentPage(
        { ...EMPTY_FILTERS, app: app.name },
        '',
      )
      return page.items.find(canRollbackTo) ?? null
    },
    enabled: asking === 'rollback',
  })

  const tone = TONE[VARIANT_TONE[app.status.variant]]
  const deployedAt = app.summary?.last_deploy_at
  const ready = app.summary?.ready
  const canRestart = can('write') || can('deploy')
  const canDeploy = can('deploy')

  function confirmRestart() {
    restart.mutate(app.name, {
      onSuccess: () => {
        toast.add({
          title: t('phoneHome.restarting', { name: app.name }),
          type: 'success',
        })
        setAsking(null)
      },
      onError: (error) => {
        toast.add({ title: error.message, type: 'error' })
        setAsking(null)
      },
    })
  }

  function confirmRollback() {
    if (!target.data) return
    rollback.mutate(target.data, {
      onSuccess: (result) => {
        toast.add({
          title: t(
            isPendingApproval(result)
              ? 'phoneHome.rollbackWaiting'
              : 'phoneHome.rollingBack',
            { name: app.name },
          ),
          type: 'success',
        })
        setAsking(null)
      },
      onError: (error) => {
        toast.add({ title: error.message, type: 'error' })
        setAsking(null)
      },
    })
  }

  const healthKey =
    ready === 'true' ? 'ready' : ready === 'false' ? 'notReady' : 'unknown'
  const when = target.data ? formatRelativeTime(target.data.started_at) : ''

  return (
    <li className="rounded-lg border border-border bg-card p-3">
      <div className="flex items-start justify-between gap-3">
        <Link
          to="/apps/$name"
          params={{ name: app.name }}
          className="flex min-h-11 min-w-0 items-center truncate text-sm font-medium text-foreground"
        >
          {app.name}
        </Link>
        <span
          className={cn(
            'inline-flex min-h-11 shrink-0 items-center gap-1.5 text-xs font-medium',
            tone.text,
          )}
        >
          <span
            aria-hidden="true"
            className={cn('size-2 rounded-full', tone.solid)}
          />
          {app.status.label}
        </span>
      </div>
      <p className="text-xs text-muted-foreground">
        {deployedAt
          ? t('phoneHome.lastDeploy', { when: formatRelativeTime(deployedAt) })
          : t('phoneHome.neverDeployed')}
        {' · '}
        {t(`phoneHome.health.${healthKey}`)}
      </p>
      {canRestart || canDeploy ? (
        <div className="mt-2 flex gap-2">
          {canRestart ? (
            <Button
              type="button"
              variant="outline"
              className={ACTION_BTN}
              onClick={() => setAsking('restart')}
            >
              <RotateCwGlassIcon size="1.4em" />
              {t('phoneHome.restart')}
            </Button>
          ) : null}
          {canDeploy ? (
            <>
              <Button
                type="button"
                variant="outline"
                className={ACTION_BTN}
                onClick={() => setAsking('redeploy')}
              >
                <RocketGlassIcon size="1.4em" />
                {t('phoneHome.redeploy')}
              </Button>
              <Button
                type="button"
                variant="outline"
                className={ACTION_BTN}
                onClick={() => setAsking('rollback')}
              >
                <RestoreGlassIcon size="1.4em" />
                {t('phoneHome.rollback')}
              </Button>
            </>
          ) : null}
        </div>
      ) : null}
      <ConfirmDialog
        open={asking === 'restart'}
        onOpenChange={(o) => !o && setAsking(null)}
        title={t('phoneHome.restartTitle', { name: app.name })}
        description={t('phoneHome.restartBody')}
        confirmLabel={t('phoneHome.restart')}
        cancelLabel={t('phoneHome.cancel')}
        pending={restart.isPending}
        onConfirm={confirmRestart}
      />
      <ConfirmDialog
        open={asking === 'redeploy'}
        onOpenChange={(o) => !o && setAsking(null)}
        title={t('phoneHome.redeployTitle', { name: app.name })}
        description={t('phoneHome.redeployBody')}
        confirmLabel={t('phoneHome.redeploy')}
        cancelLabel={t('phoneHome.cancel')}
        pending={redeploying}
        onConfirm={redeploy}
      />
      <ConfirmDialog
        open={asking === 'rollback'}
        onOpenChange={(o) => !o && setAsking(null)}
        title={t('phoneHome.rollbackTitle', { name: app.name })}
        description={
          target.isPending
            ? ''
            : target.data
              ? t('phoneHome.rollbackBody', { when })
              : t('phoneHome.rollbackNone')
        }
        confirmLabel={t('phoneHome.rollback')}
        cancelLabel={t('phoneHome.cancel')}
        confirmDisabled={!target.data}
        pending={rollback.isPending}
        onConfirm={confirmRollback}
      />
    </li>
  )
}
