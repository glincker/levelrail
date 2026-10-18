import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { useBulkApps, type BulkResponse } from '@/queries/appsBulk'

export type BulkAppAction = 'restart' | 'redeploy'

const MAX_LISTED = 8
const BLOCKED = ['denied', 'not_found', 'error']

function badgeTone(
  status: string,
): 'success' | 'warning' | 'destructive' | 'muted' {
  if (status === 'ok' || status === 'would_apply') return 'success'
  if (status === 'skipped') return 'muted'
  return status === 'denied' ? 'warning' : 'destructive'
}

function Summary({ result }: { result: BulkResponse }) {
  const { t } = useTranslation('resources')
  const skipped = result.counts.skipped ?? 0
  const blocked = BLOCKED.reduce((n, k) => n + (result.counts[k] ?? 0), 0)
  const changed = result.dry_run
    ? (result.counts.would_apply ?? 0)
    : (result.counts.ok ?? 0)
  const attention = result.results.filter(
    (r) => BLOCKED.includes(r.status) || r.status === 'skipped',
  )
  return (
    <div className="space-y-2" aria-live="polite">
      <p className="text-sm text-foreground">
        {result.dry_run
          ? t('appAction.summaryDry', { would: changed, skipped, blocked })
          : t('appAction.summaryDone', { done: changed, skipped, blocked })}
      </p>
      {attention.length > 0 ? (
        <ul className="space-y-1 text-sm">
          {attention.slice(0, MAX_LISTED).map((r) => (
            <li
              key={r.name}
              className="flex items-center justify-between gap-2"
            >
              <span className="truncate">{r.name}</span>
              <Badge variant={badgeTone(r.status)}>
                {t(`status.${r.status}`)}
              </Badge>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

// Two steps on purpose: apply only unlocks after a server dry run reports something would change.
export function BulkAppActionDialog({
  open,
  onOpenChange,
  action,
  names,
  onDone,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  action: BulkAppAction
  names: string[]
  onDone: () => void
}) {
  const { t } = useTranslation('resources')
  const mutation = useBulkApps()
  const [previewed, setPreviewed] = useState<BulkResponse | null>(null)
  const [done, setDone] = useState<BulkResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const wouldRun = previewed?.counts.would_apply ?? 0
  const canApply = wouldRun > 0 && !done

  function reset(next: boolean) {
    if (!next) {
      setPreviewed(null)
      setDone(null)
      setError(null)
    }
    onOpenChange(next)
  }

  function run(dryRun: boolean) {
    setError(null)
    mutation.mutate(
      { action, names, dry_run: dryRun },
      {
        onSuccess: (res) => {
          if (dryRun) {
            setPreviewed(res)
            return
          }
          setDone(res)
          toast.add({
            title: t(`appAction.${action}Toast`, {
              count: res.counts.ok ?? 0,
            }),
            type: 'success',
          })
          onDone()
        },
        onError: (err) =>
          setError(err instanceof Error ? err.message : t('appAction.failed')),
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={reset}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t(`appAction.${action}Title`, { count: names.length })}
          </DialogTitle>
          <DialogDescription>
            {t(`appAction.${action}Description`)}
          </DialogDescription>
        </DialogHeader>
        {done ? <Summary result={done} /> : null}
        {!done && previewed ? <Summary result={previewed} /> : null}
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => reset(false)}>
            {done ? t('dialog.close') : t('dialog.cancel')}
          </Button>
          {done ? null : (
            <>
              <Button
                variant="secondary"
                disabled={mutation.isPending}
                onClick={() => run(true)}
              >
                {mutation.isPending && !canApply
                  ? t('dialog.previewing')
                  : t('dialog.preview')}
              </Button>
              <Button
                disabled={!canApply || mutation.isPending}
                onClick={() => run(false)}
              >
                {mutation.isPending && canApply
                  ? t('dialog.applying')
                  : t('dialog.apply', { count: wouldRun })}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
