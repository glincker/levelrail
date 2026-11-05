import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleXGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import {
  attachDropsSettings,
  decodeTarget,
  encodeTarget,
  moveDropsSettings,
  type TargetOption,
} from '../../lib/domainLifecycle'
import {
  attachItem,
  isNoopBatchMove,
  itemsNeedingConfirmation,
  mergeResults,
  moveItem,
  parkItem,
  resultState,
  summarize,
  type BatchAction,
  type BatchItem,
  type BatchItemResult,
} from '../../lib/domainsBatch'
import { useDomainsBatch } from '../../queries/domainsBatch'
import { EnvironmentTargetSelect, TargetSelect } from './DomainLifecycleDialogs'
import { firstTarget, useTargetGroups } from './useTargetGroups'

// One selected row, live or parked.
export interface BatchSubject {
  domain: string
  service_name: string
  environment_id?: string
  has_settings?: boolean
}

const NO_SOURCE: TargetOption = { name: '', kind: 'app' }

function previewText(
  t: TFunction<'domains'>,
  action: BatchAction,
  s: BatchSubject,
  item: BatchItem,
  noop: boolean,
): string {
  if (noop) return t('bulk.preview.noop')
  if (action === 'park') return t('bulk.preview.park', { app: s.service_name })
  if (action === 'move') {
    return t('bulk.preview.move', { from: s.service_name, to: item.to })
  }
  if (action === 'attach') return t('bulk.preview.attach', { app: item.app })
  return t('bulk.preview.delete')
}

function ResultIcon({ state }: { state: ReturnType<typeof resultState> }) {
  const { t } = useTranslation('domains')
  if (state === 'ok') {
    return (
      <CircleCheckGlassIcon
        size="1.4em"
        className="text-tone-success"
        aria-label={t('bulk.status.ok')}
      />
    )
  }
  if (state === 'needs_confirmation') {
    return (
      <CircleAlertGlassIcon
        size="1.4em"
        className="text-tone-warning"
        aria-label={t('bulk.status.needsConfirmation')}
      />
    )
  }
  return (
    <CircleXGlassIcon
      size="1.4em"
      className="text-destructive"
      aria-label={t('bulk.status.failed')}
    />
  )
}

function resultText(t: TFunction<'domains'>, r: BatchItemResult): string {
  const state = resultState(r)
  if (state === 'failed') {
    return r.error || r.message || r.code || t('bulk.status.failedGeneric')
  }
  if (state === 'needs_confirmation') {
    return r.message || t('bulk.status.needsConfirmationBody')
  }
  return r.message || t('bulk.status.okBody')
}

export function BatchResultsTable({ results }: { results: BatchItemResult[] }) {
  const { t } = useTranslation('domains')
  return (
    <table className="w-full text-sm">
      <caption className="sr-only">{t('bulk.resultsLabel')}</caption>
      <thead className="sr-only">
        <tr>
          <th scope="col">{t('bulk.col.domain')}</th>
          <th scope="col">{t('bulk.col.result')}</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-border">
        {results.map((r) => (
          <tr key={r.domain}>
            <td className="py-1.5 pr-2 align-top font-mono">{r.domain}</td>
            <td className="py-1.5 align-top">
              <span className="flex items-start gap-1.5">
                <ResultIcon state={resultState(r)} />
                <span className="text-muted-foreground">
                  {resultText(t, r)}
                </span>
              </span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// Configure and preview, run in chunks, then show per-item results. Move asks
// the server for a collision check first; Confirm these resends only those.
export function DomainsBatchDialog({
  action,
  subjects,
  open,
  onOpenChange,
  onDone,
}: {
  action: BatchAction
  subjects: BatchSubject[]
  open: boolean
  onOpenChange: (open: boolean) => void
  // Called on close with the domains that succeeded, to drop them from selection.
  onDone: (okDomains: string[]) => void
}) {
  const { t } = useTranslation('domains')
  const needsTarget = action === 'move' || action === 'attach'
  const groups = useTargetGroups(NO_SOURCE, true)
  const [to, setTo] = useState('')
  const [env, setEnv] = useState('')
  const [discard, setDiscard] = useState(false)
  const [results, setResults] = useState<BatchItemResult[] | null>(null)
  const batch = useDomainsBatch()

  const chosen = decodeTarget(to) ?? firstTarget(groups)
  const toStatic = chosen?.kind === 'static_site'
  const anySettings = subjects.some((s) => s.has_settings === true)

  const rows = useMemo(
    () =>
      subjects.map((s) => {
        let item: BatchItem = { domain: s.domain }
        if (action === 'park') item = parkItem(s)
        else if (action === 'move' && chosen) item = moveItem(s, chosen, env)
        else if (action === 'attach' && chosen) {
          item = attachItem(s.domain, chosen, env)
        }
        const noop =
          action === 'move' && chosen !== null && isNoopBatchMove(item)
        return { subject: s, item, noop }
      }),
    [subjects, action, chosen, env],
  )
  const sendable = rows.filter((r) => !r.noop).map((r) => r.item)
  const dropsMove = action === 'move' && moveDropsSettings('app', chosen?.kind)
  const dropsAttach =
    action === 'attach' && attachDropsSettings(anySettings, chosen?.kind)
  const summary = results ? summarize(results) : null
  const waiting = summary?.requires_confirmation ?? 0

  function run(items: BatchItem[], confirm: boolean) {
    batch.mutate(
      {
        action,
        items,
        confirm: confirm || action === 'delete_parked',
        ...(action === 'attach' ? { discard_settings: discard } : {}),
      },
      {
        onSuccess: (got) => {
          setResults((prev) => mergeResults(prev ?? [], got))
        },
      },
    )
  }

  function close(next: boolean) {
    if (!next) {
      const ok = (results ?? [])
        .filter((r) => r.ok === true)
        .map((r) => r.domain)
      if (ok.length > 0) onDone(ok)
      setResults(null)
    }
    onOpenChange(next)
  }

  const title = t(`bulk.title.${action}`, { count: subjects.length })
  const confirmLabel = t(`bulk.confirm.${action}`, { count: sendable.length })
  const disabled =
    sendable.length === 0 || (needsTarget && chosen === null) || batch.isPending

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{t(`bulk.body.${action}`)}</DialogDescription>
        </DialogHeader>

        {results === null ? (
          <>
            {needsTarget ? (
              <div className="space-y-2">
                <TargetSelect
                  id="bulk-target"
                  label={
                    action === 'move'
                      ? t('lifecycle.move.target')
                      : t('bulk.attachTo')
                  }
                  value={chosen ? encodeTarget(chosen) : ''}
                  groups={groups}
                  onChange={(v) => {
                    setTo(v)
                    setEnv('')
                  }}
                />
                {toStatic || !chosen ? null : (
                  <EnvironmentTargetSelect
                    id="bulk-target-environment"
                    app={chosen.name}
                    value={env}
                    onChange={setEnv}
                  />
                )}
              </div>
            ) : null}
            {action === 'park' ? (
              <p className="text-sm text-muted-foreground">
                {t('bulk.parkKeeps')}
              </p>
            ) : null}
            {action === 'attach' && anySettings && !toStatic ? (
              <div className="flex items-center gap-2">
                <Checkbox
                  id="bulk-discard"
                  checked={discard}
                  onCheckedChange={setDiscard}
                />
                <Label htmlFor="bulk-discard">{t('bulk.discard')}</Label>
              </div>
            ) : null}
            {dropsMove || dropsAttach ? (
              <p role="alert" className="text-sm text-tone-warning">
                {dropsMove
                  ? t('lifecycle.move.dropsWarning')
                  : t('lifecycle.attach.dropsWarning')}
              </p>
            ) : null}
            <ul
              aria-label={t('bulk.previewLabel')}
              className="max-h-56 divide-y divide-border overflow-auto rounded-md border border-border text-sm"
            >
              {rows.map(({ subject, item, noop }) => (
                <li
                  key={subject.domain}
                  className="flex flex-wrap items-baseline justify-between gap-x-3 px-2.5 py-1.5"
                >
                  <span className="font-mono">{subject.domain}</span>
                  <span className="text-xs text-muted-foreground">
                    {previewText(t, action, subject, item, noop)}
                  </span>
                </li>
              ))}
            </ul>
          </>
        ) : (
          <div className="space-y-2">
            {summary ? (
              <p role="status" className="text-sm font-medium">
                {t('bulk.summary', {
                  ok: summary.succeeded,
                  failed: summary.failed,
                  waiting: summary.requires_confirmation,
                })}
              </p>
            ) : null}
            <div className="max-h-64 overflow-auto rounded-md border border-border px-2.5">
              <BatchResultsTable results={results} />
            </div>
          </div>
        )}

        {batch.error ? (
          <Alert variant="destructive">
            <AlertDescription>{batch.error.message}</AlertDescription>
          </Alert>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="max-sm:min-h-11"
            onClick={() => {
              close(false)
            }}
          >
            {results === null ? t('bulk.cancel') : t('bulk.done')}
          </Button>
          {results === null ? (
            <Button
              type="button"
              variant={action === 'delete_parked' ? 'destructive' : 'default'}
              className="max-sm:min-h-11"
              disabled={disabled}
              onClick={() => {
                run(sendable, false)
              }}
            >
              {batch.isPending ? t('lifecycle.pending') : confirmLabel}
            </Button>
          ) : waiting > 0 ? (
            <Button
              type="button"
              className="max-sm:min-h-11"
              disabled={batch.isPending}
              onClick={() => {
                run(itemsNeedingConfirmation(sendable, results), true)
              }}
            >
              {batch.isPending
                ? t('lifecycle.pending')
                : t('bulk.confirmThese', { count: waiting })}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
