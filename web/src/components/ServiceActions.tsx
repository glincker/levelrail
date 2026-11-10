import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import {
  MinusGlassIcon,
  PlusGlassIcon,
  RotateCcwGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { useAbilities } from '../hooks/useAbilities'
import { fetchApp, useRestartApp, useUpdateApp } from '../queries/apps'
import { invalidateAppGroup } from '../queries/appGroup'
import type { AppDetail } from '../types/appDetail'

// Per-service replica stepper and restart for a multi-service group row.
// Scale refetches the service (PUT is a full replace) so a stale group copy
// never overwrites newer fields; the server alone validates the range.
export function ServiceActions({
  svc,
  appName,
}: {
  svc: AppDetail
  appName: string
}) {
  const { t } = useTranslation('appServices')
  const queryClient = useQueryClient()
  const { can } = useAbilities()
  const canDeploy = can('deploy')
  const errId = useId()
  const current = svc.replicas ?? 1
  const [next, setNext] = useState(current)
  const [error, setError] = useState<string | null>(null)
  const [confirm, setConfirm] = useState(false)
  const update = useUpdateApp(svc.name, { inlineError: true })
  const restart = useRestartApp()
  const readOnlyTitle = canDeploy ? undefined : t('readOnly')

  async function apply() {
    setError(null)
    try {
      const fresh = await fetchApp(svc.name)
      await update.mutateAsync({ ...fresh, replicas: next })
      await invalidateAppGroup(queryClient, appName)
    } catch (e) {
      setError(e instanceof Error ? e.message : t('scaleFailed'))
    }
  }

  function runRestart() {
    restart.mutate(svc.name, {
      onSuccess: () => {
        setConfirm(false)
        void invalidateAppGroup(queryClient, appName)
      },
    })
  }

  return (
    <div className="relative z-10 flex flex-col gap-1">
      <div className="flex items-center gap-1" title={readOnlyTitle}>
        <Button
          type="button"
          variant="outline"
          size="icon-xs"
          aria-label={t('decrease', { name: svc.name })}
          disabled={!canDeploy || update.isPending}
          onClick={() => setNext((n) => n - 1)}
        >
          <MinusGlassIcon aria-hidden="true" />
        </Button>
        <input
          type="number"
          inputMode="numeric"
          aria-label={`${t('replicas')} ${svc.name}`}
          aria-describedby={error ? errId : undefined}
          aria-invalid={error ? true : undefined}
          value={next}
          disabled={!canDeploy || update.isPending}
          onChange={(e) => setNext(Number(e.target.value))}
          className="h-6 w-12 rounded-md border border-input bg-transparent px-1 text-center text-xs tabular-nums"
        />
        <Button
          type="button"
          variant="outline"
          size="icon-xs"
          aria-label={t('increase', { name: svc.name })}
          disabled={!canDeploy || update.isPending}
          onClick={() => setNext((n) => n + 1)}
        >
          <PlusGlassIcon aria-hidden="true" />
        </Button>
        <Button
          type="button"
          size="xs"
          disabled={!canDeploy || update.isPending || next === current}
          onClick={() => void apply()}
        >
          {update.isPending ? t('applying') : t('apply')}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="icon-xs"
          aria-label={t('restart', { name: svc.name })}
          title={readOnlyTitle ?? t('restart', { name: svc.name })}
          disabled={!canDeploy}
          onClick={() => setConfirm(true)}
        >
          <RotateCcwGlassIcon aria-hidden="true" />
        </Button>
      </div>
      {error ? (
        <p id={errId} role="alert" className="text-destructive text-xs">
          {error}
        </p>
      ) : null}
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t('restartTitle', { name: svc.name })}
        description={t('restartBody')}
        confirmLabel={t('restartConfirm')}
        pendingLabel={t('restarting')}
        pending={restart.isPending}
        error={restart.isError ? restart.error.message : null}
        onConfirm={runRestart}
      />
    </div>
  )
}
