import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  ExternalLinkGlassIcon,
  EyeGlassIcon,
  LockGlassIcon,
  TrashGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { formatRelative } from '../kit/formatRelative'
import { StatusPill } from '../kit/StatusPill'
import type { Tone } from '../kit/tone'
import { RetentionDaysSelect } from '../settings/RetentionDaysSelect'
import { formatSize } from '../../lib/format'
import {
  CATEGORY_STYLE,
  DEFAULT_PRESETS,
  isReadThrough,
  isShortening,
  percentOf,
  retentionOptions,
} from '../../lib/cleanup'
import {
  usePreviewCleanup,
  useRunCleanup,
  useSetCleanupMode,
} from '../../queries/cleanup'
import { useSetRuntimeSetting } from '../../queries/runtimeSettings'
import type {
  CleanupImpact,
  CleanupMode,
  CleanupResult,
  CleanupTarget,
} from '../../types/cleanup'
import { ModeControl } from './ModeControl'
import { UsageBar } from './UsageBar'

const MODE_TONE: Record<CleanupMode, Tone> = {
  on: 'success',
  preview: 'warning',
  off: 'neutral',
}

export function CleanupTargetRow({
  target,
  categoryBytes,
  canEdit,
  allowedDays,
}: {
  target: CleanupTarget
  categoryBytes: number
  canEdit: boolean
  allowedDays?: number[]
}) {
  const { t } = useTranslation('cleanup')
  const preview = usePreviewCleanup()
  const run = useRunCleanup()
  const setMode = useSetCleanupMode()
  const setRetention = useSetRuntimeSetting()
  const [impact, setImpact] = useState<CleanupImpact | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [result, setResult] = useState<CleanupResult | null>(null)
  const [confirmRun, setConfirmRun] = useState(false)
  const [confirmMode, setConfirmMode] = useState(false)
  const [confirmShorten, setConfirmShorten] = useState<number | null>(null)
  const [flashes, setFlashes] = useState(0)

  const readThrough = isReadThrough(target)
  const running = run.isPending
  const status: CleanupMode | 'running' = running ? 'running' : target.mode
  const days = target.days
  const nothingToKeep = days === 0
  const share = percentOf(target.usage.bytes ?? 0, categoryBytes)
  const style = CATEGORY_STYLE[target.category]

  function saveRetention(next: number, confirmWeakening = false) {
    setRetention.mutate(
      { key: target.policy_key, value: String(next), confirmWeakening },
      {
        onSuccess: () => {
          setConfirmShorten(null)
          setImpact(null)
          toast.add({
            type: 'success',
            title: t('toast.retentionSaved', { title: target.title }),
          })
        },
        onError: (err) => {
          setConfirmShorten(null)
          toast.add({
            type: 'error',
            title: t('toast.retentionError', {
              title: target.title,
              message: err.message,
            }),
          })
        },
      },
    )
  }

  function changeRetention(next: number) {
    if (next === days) return
    if (target.destructive && isShortening(days, next)) {
      setConfirmShorten(next)
      return
    }
    saveRetention(next)
  }

  function applyMode(next: CleanupMode) {
    setMode.mutate(
      { id: target.id, mode: next },
      {
        onSuccess: () => {
          setConfirmMode(false)
          toast.add({
            type: 'success',
            title: t('toast.modeSaved', {
              title: target.title,
              mode: t(`target.modes.${next}`),
            }),
          })
        },
        onError: (err) => {
          setConfirmMode(false)
          toast.add({
            type: 'error',
            title: t('toast.modeError', {
              title: target.title,
              message: err.message,
            }),
          })
        },
      },
    )
  }

  function changeMode(next: CleanupMode) {
    if (next === 'on' && (target.origin === 'new' || target.destructive)) {
      setConfirmMode(true)
      return
    }
    applyMode(next)
  }

  function fetchPreview(then?: () => void) {
    setResult(null)
    setPreviewError(null)
    preview.mutate(
      { id: target.id, days },
      {
        onSuccess: (r) => {
          setImpact(r)
          then?.()
        },
        onError: (err) => {
          setImpact(null)
          setPreviewError(err.message)
        },
      },
    )
  }

  function startClean() {
    if (impact) setConfirmRun(true)
    else fetchPreview(() => setConfirmRun(true))
  }

  function confirmClean() {
    setConfirmRun(false)
    setImpact(null)
    setResult(null)
    run.mutate(
      { id: target.id, days },
      {
        onSuccess: (r) => {
          setResult(r)
          if (!r.error) setFlashes((n) => n + 1)
        },
        onError: (err) => {
          setResult({ rows: 0, bytes: 0, error: err.message })
        },
      },
    )
  }

  const last = target.last_run
  const lastText = last
    ? t(
        last.error
          ? 'target.lastRunFailed'
          : last.dry_run
            ? 'target.lastRunDry'
            : 'target.lastRun',
        { when: formatRelative(last.started_at) },
      )
    : t('target.neverRun')

  const ownerHint = canEdit ? undefined : t('readOnly')

  return (
    <li
      data-target={target.id}
      className="relative border-b border-border px-3 py-3 last:border-b-0 sm:px-4"
    >
      {flashes > 0 ? (
        <span
          key={flashes}
          aria-hidden="true"
          className="cleanup-flash pointer-events-none absolute inset-0 bg-tone-success-soft"
        />
      ) : null}
      <div className="relative grid gap-x-4 gap-y-2.5 md:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_auto_auto] md:items-center">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <h3 className="text-sm font-medium text-foreground">
              {target.title}
            </h3>
            <StatusPill
              size="sm"
              tone={running ? 'info' : MODE_TONE[target.mode]}
              live={running}
              label={t(`target.status.${status}`)}
            />
            {target.origin === 'new' && target.mode === 'preview' ? (
              <Badge variant="warning">{t('target.previewBadge')}</Badge>
            ) : null}
            {target.destructive ? (
              <Tooltip>
                <TooltipTrigger
                  render={
                    <span
                      tabIndex={0}
                      aria-label={t('target.destructive')}
                      className="inline-flex size-5 items-center justify-center rounded text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <TrashGlassIcon className="size-5" aria-hidden="true" />
                    </span>
                  }
                />
                <TooltipContent>{t('target.destructive')}</TooltipContent>
              </Tooltip>
            ) : null}
          </div>
          <p className="text-xs text-muted-foreground">{target.description}</p>
          <p className="text-xs text-muted-foreground">
            {lastText}
            <span aria-hidden="true">{' · '}</span>
            {target.next_run
              ? t('target.nextRun', { when: formatRelative(target.next_run) })
              : t('target.noNextRun')}
          </p>
        </div>

        <div className="min-w-0 space-y-1">
          <p className="text-xs text-foreground tabular-nums">
            {target.usage.rows > 0 || (target.usage.bytes ?? 0) > 0
              ? t('target.usage', {
                  count: target.usage.rows,
                  rows: target.usage.rows.toLocaleString(),
                  size:
                    target.usage.bytes === null
                      ? t('target.unknown')
                      : formatSize(target.usage.bytes),
                })
              : t('target.usageEmpty')}
            {target.usage.rows > 0 || target.usage.oldest ? (
              <span className="text-muted-foreground">
                {' · '}
                {t('target.oldest', {
                  when: target.usage.oldest
                    ? formatRelative(target.usage.oldest)
                    : t('target.unknown'),
                })}
              </span>
            ) : null}
          </p>
          <UsageBar
            className="h-1"
            label={t('target.shareLabel', {
              title: target.title,
              percent: Math.round(share),
              category: t(`categories.${target.category}`),
            })}
            segments={[
              {
                key: target.id,
                percent: share,
                tone: style.tone,
                faded: style.faded,
              },
            ]}
          />
        </div>

        {readThrough ? (
          <div className="flex items-center gap-2 md:col-span-2 md:justify-end">
            <span className="text-xs text-muted-foreground">
              {t('target.readThrough')}
            </span>
            <Button
              size="sm"
              variant="outline"
              render={<Link to="/settings/cleanup" search={{ tab: 'disk' }} />}
            >
              {t('target.readThroughAction')}
              <ExternalLinkGlassIcon size="1.4em" aria-hidden="true" />
            </Button>
          </div>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2 md:flex-col md:items-start md:gap-1.5">
              <div className="flex items-center gap-1.5">
                <RetentionDaysSelect
                  value={days}
                  allowed={retentionOptions(
                    allowedDays ?? DEFAULT_PRESETS,
                    days,
                  )}
                  floor={target.floor}
                  lockedHint={t('target.lockedOption', { days: target.floor })}
                  disabled={!canEdit || setRetention.isPending}
                  ariaLabel={`${t('target.retention')}: ${target.title}`}
                  onChange={changeRetention}
                />
                {target.floor > 0 ? (
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <span
                          tabIndex={0}
                          aria-label={t('target.floor', { days: target.floor })}
                          className="inline-flex h-6 items-center gap-1 rounded text-xs text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                          <LockGlassIcon
                            className="size-4.5"
                            aria-hidden="true"
                          />
                          {target.floor}
                        </span>
                      }
                    />
                    <TooltipContent>
                      {t('target.floorTip', { days: target.floor })}
                    </TooltipContent>
                  </Tooltip>
                ) : null}
              </div>
              <ModeControl
                value={target.mode}
                disabled={!canEdit || setMode.isPending}
                ariaLabel={`${t('target.mode')}: ${target.title}`}
                onChange={changeMode}
              />
            </div>
            <div className="flex flex-wrap items-center gap-2 md:flex-col md:items-stretch md:gap-1.5">
              <Button
                size="sm"
                variant="outline"
                title={ownerHint}
                disabled={
                  !canEdit || nothingToKeep || running || preview.isPending
                }
                onClick={() => {
                  fetchPreview()
                }}
              >
                <EyeGlassIcon size="1.4em" aria-hidden="true" />
                {preview.isPending
                  ? t('target.previewing')
                  : t('target.preview')}
              </Button>
              <Button
                size="sm"
                variant="destructive"
                title={ownerHint}
                disabled={
                  !canEdit || nothingToKeep || running || preview.isPending
                }
                onClick={startClean}
              >
                <TrashGlassIcon size="1.4em" aria-hidden="true" />
                {t('target.cleanNow')}
              </Button>
            </div>
          </>
        )}
      </div>

      <div aria-live="polite" className="relative">
        {nothingToKeep && !readThrough && canEdit ? (
          <p className="mt-2 text-xs text-muted-foreground">
            {t('target.keepForever')}
          </p>
        ) : null}
        {running ? (
          <div className="mt-3 space-y-1.5">
            <p className="text-xs text-muted-foreground">{t('run.running')}</p>
            <div className="h-1 overflow-hidden rounded-full bg-muted">
              <div className="cleanup-sheen h-full w-1/3 bg-tone-info-solid" />
            </div>
          </div>
        ) : null}
        {impact && !running ? (
          <div className="mt-3 flex flex-wrap items-start justify-between gap-2 rounded-md border border-border bg-muted/40 px-3 py-2">
            <div className="min-w-0 space-y-0.5">
              <p className="text-xs font-medium text-muted-foreground">
                {t('impact.title')}
              </p>
              <p className="text-sm text-foreground">
                {impact.rows > 0
                  ? t('impact.summary', {
                      count: impact.rows,
                      rows: impact.rows.toLocaleString(),
                      size: formatSize(impact.bytes),
                      days,
                    })
                  : t('impact.nothing', { days })}
              </p>
            </div>
            <Button
              size="icon-xs"
              variant="ghost"
              aria-label={t('impact.close')}
              onClick={() => {
                setImpact(null)
              }}
            >
              <XGlassIcon size="1.4em" aria-hidden="true" />
            </Button>
          </div>
        ) : null}
        {previewError ? (
          <p className="mt-2 flex items-center gap-1.5 text-xs text-destructive">
            <CircleAlertGlassIcon
              className="size-5 text-tone-danger"
              aria-hidden="true"
            />
            {t('impact.error', { message: previewError })}
          </p>
        ) : null}
        {result ? (
          <div
            className={cn(
              'mt-3 flex flex-wrap items-start justify-between gap-2 rounded-md border px-3 py-2',
              result.error
                ? 'border-tone-danger-border bg-tone-danger-soft'
                : 'border-tone-success-border bg-tone-success-soft',
            )}
          >
            <p className="flex items-start gap-1.5 text-sm text-foreground">
              {result.error ? (
                <CircleAlertGlassIcon
                  className="mt-0.5 size-5 shrink-0 text-tone-danger"
                  aria-hidden="true"
                />
              ) : (
                <CircleCheckGlassIcon
                  className="mt-0.5 size-5 shrink-0 text-tone-success"
                  aria-hidden="true"
                />
              )}
              <span>
                {result.error
                  ? t('run.failed', { message: result.error })
                  : result.rows === 0
                    ? t('run.nothing')
                    : t('run.done', {
                        count: result.rows,
                        rows: result.rows.toLocaleString(),
                        size: formatSize(result.bytes),
                      })}
                {result.more_remaining ? ` ${t('run.more')}` : ''}
              </span>
            </p>
            <Button
              size="xs"
              variant="ghost"
              onClick={() => {
                setResult(null)
              }}
            >
              {t('run.dismiss')}
            </Button>
          </div>
        ) : null}
      </div>

      <ConfirmDialog
        open={confirmRun}
        onOpenChange={setConfirmRun}
        title={t('confirmRun.title', { title: target.title })}
        description={t('confirmRun.body', {
          count: impact?.rows ?? 0,
          rows: (impact?.rows ?? 0).toLocaleString(),
          size: formatSize(impact?.bytes ?? 0),
          days,
        })}
        consequences={[t('confirmRun.consequence')]}
        confirmLabel={t('confirmRun.action')}
        cancelLabel={t('confirmRun.cancel')}
        tone="destructive"
        onConfirm={confirmClean}
      />
      <ConfirmDialog
        open={confirmMode}
        onOpenChange={setConfirmMode}
        title={t('confirmMode.title', { title: target.title })}
        description={t('confirmMode.body', { days })}
        consequences={[t('confirmMode.consequence')]}
        confirmLabel={t('confirmMode.action')}
        cancelLabel={t('confirmMode.cancel')}
        tone="destructive"
        pending={setMode.isPending}
        onConfirm={() => {
          applyMode('on')
        }}
      />
      <ConfirmDialog
        open={confirmShorten !== null}
        onOpenChange={(open) => {
          if (!open) setConfirmShorten(null)
        }}
        title={t('confirmShorten.title', { title: target.title })}
        description={t('confirmShorten.body')}
        consequences={[t('confirmShorten.consequence')]}
        confirmLabel={t('confirmShorten.action')}
        cancelLabel={t('confirmShorten.cancel')}
        tone="destructive"
        pending={setRetention.isPending}
        onConfirm={() => {
          if (confirmShorten !== null) saveRetention(confirmShorten, true)
        }}
      />
    </li>
  )
}
