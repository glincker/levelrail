import '@/styles/material.css'
import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import {
  PauseGlassIcon,
  PlayGlassIcon,
  RefreshCwGlassIcon,
  RocketGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  postBulk,
  type BulkAction,
  type BulkAppResult,
  type BulkResponse,
} from '../../queries/appsBulk'
import { appKeys } from '../../queries/apps'

const PRIMARY: { value: BulkAction; icon: typeof PlayGlassIcon }[] = [
  { value: 'restart', icon: RefreshCwGlassIcon },
  { value: 'redeploy', icon: RocketGlassIcon },
  { value: 'stop', icon: PauseGlassIcon },
]

// Less frequent actions stay reachable from one select so the bar keeps one row.
const MORE: { value: BulkAction; needsValue: boolean }[] = [
  { value: 'start', needsValue: false },
  { value: 'add-tag', needsValue: true },
  { value: 'remove-tag', needsValue: true },
  { value: 'set-environment', needsValue: true },
  { value: 'move-to-project', needsValue: true },
  { value: 'delete', needsValue: false },
]

type ValueAction =
  'add-tag' | 'remove-tag' | 'set-environment' | 'move-to-project'

const DISRUPTIVE = new Set<BulkAction>(['stop', 'delete'])

const STATUS_STYLE: Record<BulkAppResult['status'], string> = {
  ok: 'text-tone-success',
  would_apply: 'text-foreground',
  denied: 'text-destructive',
  not_found: 'text-destructive',
  skipped: 'text-tone-warning',
  error: 'text-destructive',
}

function ResultList({ results }: { results: BulkAppResult[] }) {
  const { t } = useTranslation('appRows')
  return (
    <ul
      aria-label={t('bulkBar.resultsLabel')}
      className="max-h-64 divide-y divide-border overflow-auto rounded-md border border-border text-sm"
    >
      {results.map((r) => (
        <li
          key={r.name}
          className="flex flex-wrap items-baseline gap-x-2 px-3 py-1.5"
        >
          <span className="min-w-0 flex-1 truncate font-medium">{r.name}</span>
          <span className={STATUS_STYLE[r.status]}>
            {t(`bulkBar.status.${r.status}`)}
          </span>
          {r.message ? (
            <span className="w-full truncate text-xs text-muted-foreground sm:w-auto sm:max-w-[45%]">
              {r.message}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

function BulkDialog({
  action,
  value,
  names,
  onClose,
}: {
  action: BulkAction
  value: string
  names: string[]
  onClose: (applied: boolean) => void
}) {
  const { t } = useTranslation('appRows')
  const queryClient = useQueryClient()
  const inputId = useId()
  const [plan, setPlan] = useState<BulkResponse | null>(null)
  const [applied, setApplied] = useState<BulkResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [typed, setTyped] = useState('')
  const [running, setRunning] = useState(false)

  useEffect(() => {
    let live = true
    postBulk({ action, names, value: value || undefined, dry_run: true })
      .then((res) => {
        if (live) setPlan(res)
      })
      .catch((err: unknown) => {
        if (live) setError(err instanceof Error ? err.message : String(err))
      })
    return () => {
      live = false
    }
  }, [action, names, value])

  const targets = (plan?.results ?? [])
    .filter((r) => r.status === 'would_apply')
    .map((r) => r.name)
  const label = t(`bulkBar.action.${action}`)
  const confirmed = typed.trim() === String(targets.length)

  const apply = async () => {
    setRunning(true)
    setError(null)
    try {
      const res = await postBulk({
        action,
        names: targets,
        value: value || undefined,
        confirm_count: targets.length,
        confirm_names: action === 'delete' ? targets : undefined,
      })
      setApplied(res)
      void queryClient.invalidateQueries({ queryKey: appKeys.all })
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setRunning(false)
    }
  }

  const failed = applied
    ? applied.results.filter((r) => r.status !== 'ok').length
    : 0

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !running) onClose(applied !== null)
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {applied
              ? t('bulkBar.resultsTitle', { action: label })
              : t('bulkBar.reviewTitle', {
                  action: label,
                  count: names.length,
                })}
          </DialogTitle>
          <DialogDescription>
            {applied
              ? t('bulkBar.summary', {
                  ok: applied.results.length - failed,
                  failed,
                })
              : t('bulkBar.reviewDescription')}
          </DialogDescription>
        </DialogHeader>
        {applied ? (
          <ResultList results={applied.results} />
        ) : plan ? (
          <>
            <ResultList results={plan.results} />
            {DISRUPTIVE.has(action) && targets.length > 0 ? (
              <p className="flex items-start gap-1.5 text-sm text-foreground">
                <TriangleAlertGlassIcon
                  className="mt-0.5 size-5 shrink-0 text-tone-warning"
                  aria-hidden="true"
                />
                {t(
                  `bulkBar.warning.${action === 'delete' ? 'delete' : 'stop'}`,
                  {
                    count: targets.length,
                  },
                )}
              </p>
            ) : null}
            {targets.length > 0 ? (
              <div className="flex flex-col gap-1">
                <label htmlFor={inputId} className="text-sm">
                  {t('bulkBar.typeCount', { count: targets.length })}
                </label>
                <Input
                  id={inputId}
                  inputMode="numeric"
                  autoComplete="off"
                  value={typed}
                  className="h-8 w-24"
                  onChange={(e) => {
                    setTyped(e.target.value)
                  }}
                />
              </div>
            ) : null}
          </>
        ) : error ? null : (
          <p className="text-sm text-muted-foreground" role="status">
            {t('bulkBar.checking')}
          </p>
        )}
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={running}
            onClick={() => {
              onClose(applied !== null)
            }}
          >
            {applied ? t('bulkBar.close') : t('bulkBar.cancel')}
          </Button>
          {applied ? null : (
            <Button
              type="button"
              variant={DISRUPTIVE.has(action) ? 'destructive' : 'default'}
              disabled={targets.length === 0 || !confirmed || running}
              onClick={() => {
                void apply()
              }}
            >
              {running
                ? t('bulkBar.working')
                : t('bulkBar.confirm', {
                    action: label,
                    count: targets.length,
                  })}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** BulkActionsBar runs restart, redeploy, stop and the other bulk verbs on the selected apps. */
export function BulkActionsBar({
  selected,
  canAct,
  onClear,
}: {
  selected: string[]
  canAct: boolean
  onClear: () => void
}) {
  const { t } = useTranslation('appRows')
  const [pending, setPending] = useState<BulkAction | null>(null)
  const [more, setMore] = useState<BulkAction | ''>('')
  const [value, setValue] = useState('')
  const count = selected.length
  const moreSpec = MORE.find((m) => m.value === more)

  return (
    <>
      <p role="status" aria-live="polite" className="sr-only">
        {count > 0 ? t('bulkBar.selected', { count }) : ''}
      </p>
      {count > 0 ? (
        <div
          role="region"
          aria-label={t('bulkBar.region')}
          data-dock-avoid=""
          className="fixed bottom-[max(1.5rem,env(safe-area-inset-bottom))] left-1/2 z-30 flex w-max max-w-[calc(100vw-1rem)] -translate-x-1/2 flex-wrap items-center justify-center gap-2 glass glass-thick rounded-xl px-3 py-2"
        >
          <span className="text-sm font-medium" aria-hidden="true">
            {t('bulkBar.selected', { count })}
          </span>
          {PRIMARY.map(({ value: v, icon: Icon }) => (
            <Button
              key={v}
              type="button"
              size="sm"
              variant={v === 'stop' ? 'outline' : 'default'}
              disabled={!canAct}
              onClick={() => {
                setValue('')
                setPending(v)
              }}
            >
              <Icon size="1.4em" aria-hidden="true" />
              {t(`bulkBar.action.${v}`)}
            </Button>
          ))}
          <select
            aria-label={t('bulkBar.more')}
            value={more}
            disabled={!canAct}
            className="h-8 rounded-md border border-input bg-background px-2 text-sm"
            onChange={(e) => {
              setMore(e.target.value as BulkAction | '')
              setValue('')
            }}
          >
            <option value="">{t('bulkBar.more')}</option>
            {MORE.map((m) => (
              <option key={m.value} value={m.value}>
                {t(`bulkBar.action.${m.value}`)}
              </option>
            ))}
          </select>
          {moreSpec?.needsValue ? (
            <Input
              value={value}
              aria-label={t(`bulkBar.value.${moreSpec.value as ValueAction}`)}
              placeholder={t(`bulkBar.value.${moreSpec.value as ValueAction}`)}
              className="h-8 w-full sm:w-44"
              onChange={(e) => {
                setValue(e.target.value)
              }}
            />
          ) : null}
          {more ? (
            <Button
              type="button"
              size="sm"
              variant={more === 'delete' ? 'destructive' : 'outline'}
              disabled={!canAct || (moreSpec?.needsValue && !value.trim())}
              onClick={() => {
                setPending(more)
              }}
            >
              {t('bulkBar.review')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="icon-sm"
            variant="ghost"
            aria-label={t('bulkBar.clear')}
            onClick={onClear}
          >
            <XGlassIcon size="1.4em" />
          </Button>
          {canAct ? null : (
            <p className="w-full text-center text-xs text-muted-foreground">
              {t('bulkBar.readOnly')}
            </p>
          )}
        </div>
      ) : null}
      {pending ? (
        <BulkDialog
          action={pending}
          value={value.trim()}
          names={selected}
          onClose={(applied) => {
            setPending(null)
            if (applied) {
              setMore('')
              onClear()
            }
          }}
        />
      ) : null}
    </>
  )
}
