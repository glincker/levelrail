import { useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import {
  CheckCircleIcon,
  CircleIcon,
  MinusCircleIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { dockerIntegrationKeys } from '../../queries/dockerIntegration'
import {
  useDatabaseProvision,
  useRetryProvision,
  type ProvisionAttempt,
  type ProvisionStep,
} from '../../queries/databaseProvision'

function elapsed(
  from: string | undefined,
  to: string | undefined,
  now: number,
) {
  if (!from) return ''
  const end = to ? Date.parse(to) : now
  const s = Math.max(0, Math.round((end - Date.parse(from)) / 1000))
  return s >= 60 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${s}s`
}

function StepIcon({ status }: { status: ProvisionStep['status'] }) {
  const cls = 'mt-0.5 size-4 shrink-0'
  switch (status) {
    case 'done':
      return (
        <CheckCircleIcon aria-hidden="true" className={`${cls} text-success`} />
      )
    case 'started':
      return (
        <SpinnerGapIcon
          aria-hidden="true"
          className={`${cls} animate-spin text-primary`}
        />
      )
    case 'failed':
      return (
        <WarningCircleIcon
          aria-hidden="true"
          className={`${cls} text-destructive`}
        />
      )
    case 'skipped':
      return (
        <MinusCircleIcon
          aria-hidden="true"
          className={`${cls} text-muted-foreground`}
        />
      )
    default:
      return (
        <CircleIcon
          aria-hidden="true"
          className={`${cls} text-muted-foreground`}
        />
      )
  }
}

// Pure view of one attempt; the container below feeds it from polling.
export function ProvisioningView({
  name,
  attempt,
  onFix,
  fixPending,
}: {
  name: string
  attempt: ProvisionAttempt
  onFix: (fix: string, confirm: boolean) => void
  fixPending?: boolean
}) {
  const { t } = useTranslation('databases')
  const [now, setNow] = useState(() => Date.now())
  const [confirming, setConfirming] = useState(false)
  const [showLog, setShowLog] = useState(false)
  const active = attempt.status === 'in_progress'
  useEffect(() => {
    if (!active) return
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [active])

  const current = attempt.steps.find((s) => s.status === 'started')
  const fix = attempt.failure?.fix

  return (
    <Card data-testid="provisioning-panel">
      <CardHeader>
        <CardTitle className="flex flex-wrap items-baseline justify-between gap-2">
          <span>
            {attempt.status === 'ready'
              ? t('provision.readyTitle')
              : attempt.status === 'failed'
                ? t('provision.failedTitle')
                : t('provision.title')}
          </span>
          <span className="text-xs font-normal text-muted-foreground">
            {t('provision.elapsed', {
              time: elapsed(attempt.started_at, attempt.finished_at, now),
            })}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="sr-only" aria-live="polite">
          {current
            ? t(`provision.steps.${current.step}` as 'provision.steps.ready')
            : ''}
        </p>
        <ol className="space-y-3">
          {attempt.steps.map((s) => (
            <li key={s.step} className="flex min-w-0 gap-2">
              <StepIcon status={s.status} />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">
                  {t(`provision.steps.${s.step}` as 'provision.steps.ready')}
                </p>
                {s.status === 'started' &&
                s.step === 'pull_image' &&
                s.total ? (
                  <div className="mt-1 space-y-1">
                    <Progress
                      value={Math.round(((s.current ?? 0) * 100) / s.total)}
                      aria-label={t('provision.pullProgress')}
                    />
                    <p className="truncate text-xs text-muted-foreground">
                      {Math.round(((s.current ?? 0) * 100) / s.total)}%
                      {s.detail ? `, ${s.detail}` : ''}
                    </p>
                  </div>
                ) : s.detail && s.status !== 'pending' ? (
                  <p className="break-words text-xs text-muted-foreground">
                    {s.detail}
                  </p>
                ) : null}
              </div>
            </li>
          ))}
        </ol>

        {attempt.failure ? (
          <div
            role="alert"
            className="space-y-2 rounded-md border border-destructive/40 p-3"
          >
            <p className="text-sm break-words text-destructive">
              {t('provision.failedAt', {
                step: t(
                  `provision.steps.${attempt.failure.step}` as 'provision.steps.ready',
                ),
              })}{' '}
              {attempt.failure.reason}
            </p>
            <div className="flex flex-wrap gap-2">
              {fix ? (
                confirming ? (
                  <>
                    <p className="w-full text-xs text-muted-foreground">
                      {t('provision.confirmRemoveVolume')}
                    </p>
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      disabled={fixPending}
                      onClick={() => {
                        onFix(fix.id, true)
                        setConfirming(false)
                      }}
                    >
                      {t('provision.confirmYes')}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setConfirming(false)
                      }}
                    >
                      {t('provision.cancel')}
                    </Button>
                  </>
                ) : (
                  <Button
                    type="button"
                    size="sm"
                    disabled={fixPending}
                    onClick={() => {
                      if (fix.confirm) setConfirming(true)
                      else onFix(fix.id, false)
                    }}
                  >
                    {fix.label}
                  </Button>
                )
              ) : (
                <Button
                  type="button"
                  size="sm"
                  disabled={fixPending}
                  onClick={() => {
                    onFix('retry', false)
                  }}
                >
                  {t('provision.retry')}
                </Button>
              )}
            </div>
          </div>
        ) : null}

        {attempt.status === 'ready' ? (
          <div className="flex flex-wrap gap-2">
            <Link
              to="/databases/$name"
              params={{ name }}
              className={buttonVariants({ size: 'sm' })}
            >
              {t('provision.open')}
            </Link>
            <Link
              to="/databases/$name/access"
              params={{ name }}
              className={buttonVariants({ size: 'sm', variant: 'outline' })}
            >
              {t('provision.connectApp')}
            </Link>
          </div>
        ) : null}

        <div>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            aria-expanded={showLog}
            onClick={() => {
              setShowLog((v) => !v)
            }}
          >
            {t('provision.details')}
          </Button>
          {showLog ? (
            <ul className="mt-2 max-h-48 space-y-1 overflow-y-auto font-mono text-xs break-words text-muted-foreground">
              {attempt.events.map((e, i) => (
                <li key={i}>
                  {new Date(e.at).toLocaleTimeString()} {e.step} {e.status}
                  {e.detail ? `: ${e.detail}` : ''}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      </CardContent>
    </Card>
  )
}

// Shows nothing for databases without a provisioning record or once ready and dismissed by the caller.
export function ProvisioningPanel({
  name,
  hideWhenReady,
}: {
  name: string
  hideWhenReady?: boolean
}) {
  const q = useDatabaseProvision(name)
  const retry = useRetryProvision(name)
  const qc = useQueryClient()
  const attempt = q.data
  if (!attempt || (hideWhenReady && attempt.status === 'ready')) return null
  return (
    <ProvisioningView
      name={name}
      attempt={attempt}
      fixPending={retry.isPending}
      onFix={(fix, confirm) => {
        if (fix === 'recheck_docker') {
          void qc.invalidateQueries({ queryKey: dockerIntegrationKeys.health })
        }
        retry.mutate({ fix, confirm })
      }}
    />
  )
}
