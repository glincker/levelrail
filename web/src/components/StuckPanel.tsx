import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CaretDownIcon, CaretRightIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { fetchApp } from '../queries/apps'
import { triggerDeploy } from '../queries/deploys'
import { useApplyFix } from '../queries/fixit'
import {
  stuckKeys,
  useStuck,
  type StuckAction,
  type StuckCause,
  type StuckKind,
} from '../queries/stuck'

function formatDuration(seconds: number): string {
  if (seconds < 90) return `${Math.max(seconds, 0)}s`
  const minutes = Math.round(seconds / 60)
  if (minutes < 120) return `${minutes}m`
  return `${Math.round(minutes / 60)}h`
}

// Deterministic "why is this not running" panel. Collapsed while the
// resource is healthy or recently non-ready, expanded once it has been
// non-ready longer than the server's APP_STUCK_THRESHOLD.
export function StuckPanel({
  kind,
  name,
  appName,
}: {
  kind: StuckKind
  name: string
  appName?: string
}) {
  const { t } = useTranslation('fixit')
  const { data, refetch, isFetching } = useStuck(kind, name)
  const [override, setOverride] = useState<boolean | null>(null)
  if (!data || data.ready) {
    return null
  }
  const open = override ?? data.stuck
  return (
    <section
      className="rounded-lg border border-border p-3 text-sm"
      data-testid="stuck-panel"
      aria-label={t('stuck.title')}
    >
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-expanded={open}
        onClick={() => {
          setOverride(!open)
        }}
      >
        {open ? <CaretDownIcon /> : <CaretRightIcon />}
        <span className="font-medium">{t('stuck.title')}</span>
        <Badge variant={data.stuck ? 'destructive' : 'warning'}>
          {t('stuck.notReadyFor', {
            duration: formatDuration(data.not_ready_seconds),
          })}
        </Badge>
      </Button>
      {open ? (
        <div className="mt-3 space-y-3">
          {data.causes.length === 0 ? (
            <p className="text-muted-foreground">{t('stuck.noCauses')}</p>
          ) : (
            data.causes.map((cause) => (
              <CauseCard
                key={cause.code}
                cause={cause}
                appName={appName ?? (kind === 'app' ? name : undefined)}
                onRecheck={() => void refetch()}
                rechecking={isFetching}
                invalidateKey={stuckKeys.detail(kind, name)}
              />
            ))
          )}
          {data.timeline.length > 0 ? (
            <div>
              <p className="text-xs font-medium text-muted-foreground">
                {t('stuck.timeline')}
              </p>
              <ul className="mt-1 space-y-1">
                {data.timeline.map((c) => (
                  <li
                    key={`${c.type}-${c.since}`}
                    className="font-mono text-xs text-muted-foreground"
                  >
                    {new Date(c.since).toLocaleString()} {c.type}={c.status} (
                    {c.reason})
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}

function CauseCard({
  cause,
  appName,
  onRecheck,
  rechecking,
  invalidateKey,
}: {
  cause: StuckCause
  appName?: string
  onRecheck: () => void
  rechecking: boolean
  invalidateKey: readonly string[]
}) {
  const { t } = useTranslation('fixit')
  return (
    <div className="rounded-lg border border-border p-3">
      <div className="flex flex-wrap items-center gap-2">
        <p className="font-medium text-foreground">{cause.title}</p>
        <Badge
          variant={cause.confidence === 'high' ? 'destructive' : 'warning'}
        >
          {cause.confidence === 'high'
            ? t('stuck.confidenceHigh')
            : t('stuck.confidenceMedium')}
        </Badge>
      </div>
      <p className="mt-1 text-muted-foreground">{cause.explanation}</p>
      {cause.evidence.length > 0 ? (
        <ul className="mt-2 space-y-1" aria-label={t('stuck.evidence')}>
          {cause.evidence.map((e) => (
            <li
              key={e}
              className="truncate font-mono text-xs text-muted-foreground"
              title={e}
            >
              {e}
            </li>
          ))}
        </ul>
      ) : null}
      <div className="mt-3 flex flex-wrap gap-2">
        {cause.actions.map((a) => (
          <ActionButton
            key={`${a.type}-${a.fix_id ?? a.href ?? ''}`}
            action={a}
            appName={appName}
            onRecheck={onRecheck}
            rechecking={rechecking}
            invalidateKey={invalidateKey}
          />
        ))}
      </div>
    </div>
  )
}

function ActionButton({
  action,
  appName,
  onRecheck,
  rechecking,
  invalidateKey,
}: {
  action: StuckAction
  appName?: string
  onRecheck: () => void
  rechecking: boolean
  invalidateKey: readonly string[]
}) {
  const { t } = useTranslation('fixit')
  const qc = useQueryClient()
  const applyFix = useApplyFix()
  const retry = useMutation({
    mutationFn: async () => {
      if (!appName) return
      const app = await fetchApp(appName)
      await triggerDeploy(appName, { image: app.image })
    },
    onSuccess: () => {
      toast.add({ title: t('stuck.retryStarted'), type: 'success' })
      void qc.invalidateQueries({ queryKey: invalidateKey })
    },
    onError: (err: Error) => {
      toast.add({
        title: t('stuck.retryFailed', { message: err.message }),
        type: 'error',
      })
    },
  })

  switch (action.type) {
    case 'fix':
      return (
        <Button
          type="button"
          size="sm"
          disabled={applyFix.isPending || !action.fix_id}
          onClick={() => {
            if (!action.fix_id) return
            applyFix.mutate(
              { id: action.fix_id },
              {
                onSuccess: () => {
                  toast.add({ title: t('stuck.fixApplied'), type: 'success' })
                  void qc.invalidateQueries({ queryKey: invalidateKey })
                },
                onError: (err) => {
                  toast.add({
                    title: t('stuck.fixFailed', { message: err.message }),
                    type: 'error',
                  })
                },
              },
            )
          }}
        >
          {applyFix.isPending ? t('applying') : action.label}
        </Button>
      )
    case 'recheck':
      return (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={rechecking}
          onClick={onRecheck}
        >
          {t('stuck.recheck')}
        </Button>
      )
    case 'retry_deploy':
      return (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={retry.isPending || !appName}
          onClick={() => {
            retry.mutate()
          }}
        >
          {retry.isPending ? t('stuck.retrying') : t('stuck.retryDeploy')}
        </Button>
      )
    default:
      return action.href ? (
        <a
          href={action.href}
          className={cn(buttonVariants({ variant: 'outline', size: 'sm' }))}
        >
          {action.label}
        </a>
      ) : null
  }
}
