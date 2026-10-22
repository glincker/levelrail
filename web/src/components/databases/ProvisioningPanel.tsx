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
  useDiscardPending,
  useRetryProvision,
  useUpdatePendingSettings,
  type PendingSettings,
  type ProvisionAttempt,
  type ProvisionFix,
  type ProvisionStep,
} from '../../queries/databaseProvision'
import { PendingSettingsForm, ReadyConnection } from './ProvisioningExtras'
import {
  connectionFor,
  type ProvisionConnection,
} from '../../lib/provisionConnection'

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
  onSaveSettings,
  onDiscard,
  connection,
  initialVersion,
}: {
  name: string
  attempt: ProvisionAttempt
  onFix: (fix: string, confirm: boolean) => void
  fixPending?: boolean
  onSaveSettings?: (s: PendingSettings) => void
  onDiscard?: () => void
  connection?: ProvisionConnection
  initialVersion?: string
}) {
  const { t } = useTranslation('databases')
  const [now, setNow] = useState(() => Date.now())
  const [confirming, setConfirming] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)
  const [discarding, setDiscarding] = useState(false)
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
  const fixes = attempt.failure?.fixes?.length
    ? attempt.failure.fixes
    : attempt.failure?.fix
      ? [attempt.failure.fix]
      : []
  const volume = attempt.failure?.volume

  function runFix(f: ProvisionFix) {
    if (f.id === 'choose_another_name') setDiscarding(true)
    else if (f.confirm) setConfirming(f.id)
    else onFix(f.id, false)
  }

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
            {volume ? (
              <p className="text-xs text-muted-foreground">
                {t('provision.leftoverVolume', {
                  name: volume.name,
                  size: volume.size_bytes
                    ? `${Math.max(1, Math.round(volume.size_bytes / 1048576))} MiB`
                    : '?',
                })}
              </p>
            ) : null}
            <div className="flex flex-wrap gap-2">
              {confirming ? (
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
                      onFix(confirming, true)
                      setConfirming(null)
                    }}
                  >
                    {t('provision.confirmYes')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setConfirming(null)
                    }}
                  >
                    {t('provision.cancel')}
                  </Button>
                </>
              ) : fixes.length > 0 ? (
                fixes.map((f) => (
                  <Button
                    key={f.id}
                    type="button"
                    size="sm"
                    variant={f.id === fixes[0]?.id ? 'default' : 'outline'}
                    disabled={fixPending}
                    onClick={() => {
                      runFix(f)
                    }}
                  >
                    {f.label}
                  </Button>
                ))
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

        {attempt.status !== 'ready' && (onSaveSettings || onDiscard) ? (
          <div className="space-y-2">
            {editing && onSaveSettings ? (
              <PendingSettingsForm
                initial={{ version: initialVersion }}
                busy={fixPending}
                onSave={(s) => {
                  onSaveSettings(s)
                  setEditing(false)
                }}
                onCancel={() => {
                  setEditing(false)
                }}
              />
            ) : discarding && onDiscard ? (
              <div className="flex flex-wrap items-center gap-2">
                <p className="w-full text-xs text-muted-foreground">
                  {t('provision.confirmDiscard')}
                </p>
                <Button
                  type="button"
                  size="sm"
                  variant="destructive"
                  onClick={onDiscard}
                >
                  {t('provision.discardYes')}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setDiscarding(false)
                  }}
                >
                  {t('provision.cancel')}
                </Button>
              </div>
            ) : (
              <div className="flex flex-wrap gap-2">
                {onSaveSettings ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setEditing(true)
                    }}
                  >
                    {t('provision.editSettings')}
                  </Button>
                ) : null}
                {onDiscard ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setDiscarding(true)
                    }}
                  >
                    {t('provision.discard')}
                  </Button>
                ) : null}
              </div>
            )}
          </div>
        ) : null}

        {attempt.status === 'ready' && connection ? (
          <ReadyConnection c={connection} />
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
  engine,
  version,
  onDiscarded,
}: {
  name: string
  hideWhenReady?: boolean
  engine?: string
  version?: string
  onDiscarded?: () => void
}) {
  const q = useDatabaseProvision(name)
  const retry = useRetryProvision(name)
  const update = useUpdatePendingSettings(name)
  const discard = useDiscardPending(name)
  const qc = useQueryClient()
  const attempt = q.data
  if (!attempt || (hideWhenReady && attempt.status === 'ready')) return null
  return (
    <ProvisioningView
      name={name}
      attempt={attempt}
      fixPending={retry.isPending || update.isPending}
      connection={connectionFor(name, engine)}
      initialVersion={version}
      onSaveSettings={(s) => {
        update.mutate(s, {
          onSuccess: () => {
            retry.mutate({})
          },
        })
      }}
      onDiscard={() => {
        discard.mutate(undefined, { onSuccess: onDiscarded })
      }}
      onFix={(fix, confirm) => {
        if (fix === 'recheck_docker') {
          void qc.invalidateQueries({ queryKey: dockerIntegrationKeys.health })
        }
        retry.mutate({ fix, confirm })
      }}
    />
  )
}
