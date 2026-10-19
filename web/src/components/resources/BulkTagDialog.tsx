import { useId, useState } from 'react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  useBulkTagResources,
  type BulkTagAction,
  type BulkTagResponse,
  type ConsoleResource,
} from '@/queries/resources'
import { useTags } from '@/queries/tags'

const MAX_LISTED = 8
const BLOCKED = ['denied', 'not_found', 'error']

function badgeTone(
  status: string,
): 'success' | 'warning' | 'destructive' | 'muted' {
  if (status === 'ok' || status === 'would_apply') return 'success'
  if (status === 'skipped') return 'muted'
  return status === 'denied' ? 'warning' : 'destructive'
}

function Preview({ result }: { result: BulkTagResponse }) {
  const { t } = useTranslation('resources')
  const skipped = result.counts.skipped ?? 0
  const blocked = BLOCKED.reduce((n, k) => n + (result.counts[k] ?? 0), 0)
  const changed = result.dry_run
    ? (result.counts.would_apply ?? 0)
    : (result.counts.ok ?? 0)
  const attention = result.results.filter((r) => BLOCKED.includes(r.status))
  return (
    <div className="space-y-2" aria-live="polite">
      <p className="text-sm text-foreground">
        {result.dry_run
          ? t('dialog.summaryDry', { would: changed, skipped, blocked })
          : t('dialog.summaryDone', { done: changed, skipped, blocked })}
      </p>
      {attention.length > 0 ? (
        <ul className="space-y-1 text-sm">
          {attention.slice(0, MAX_LISTED).map((r) => (
            <li
              key={`${r.kind}:${r.name}`}
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

// Two steps on purpose: the preview is a server dry run, apply only unlocks once it matches the tag typed.
export function BulkTagDialog({
  open,
  onOpenChange,
  action,
  selected,
  onDone,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  action: BulkTagAction
  selected: ConsoleResource[]
  onDone: () => void
}) {
  const { t } = useTranslation('resources')
  const inputId = useId()
  const listId = useId()
  const { data: tags } = useTags()
  const mutation = useBulkTagResources()
  const [tag, setTag] = useState('')
  const [previewed, setPreviewed] = useState<BulkTagResponse | null>(null)
  const [done, setDone] = useState<BulkTagResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const trimmed = tag.trim().toLowerCase()
  const previewMatches = previewed !== null && previewed.tag === trimmed
  const canApply =
    previewMatches && (previewed?.counts.would_apply ?? 0) > 0 && !done

  function reset(next: boolean) {
    if (!next) {
      setTag('')
      setPreviewed(null)
      setDone(null)
      setError(null)
    }
    onOpenChange(next)
  }

  function run(dryRun: boolean) {
    setError(null)
    mutation.mutate(
      {
        action,
        tag: trimmed,
        resources: selected.map((r) => ({ kind: r.kind, name: r.name })),
        dry_run: dryRun,
      },
      {
        onSuccess: (res) => {
          if (dryRun) {
            setPreviewed(res)
            return
          }
          setDone(res)
          toast.add({
            title: t('dialog.doneToast', {
              tag: trimmed,
              count: res.counts.ok ?? 0,
            }),
            type: 'success',
          })
          onDone()
        },
        onError: (err) =>
          setError(err instanceof Error ? err.message : t('dialog.failed')),
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={reset}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t(action === 'add' ? 'dialog.addTitle' : 'dialog.removeTitle', {
              count: selected.length,
            })}
          </DialogTitle>
          <DialogDescription>{t('dialog.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor={inputId}>{t('dialog.tagLabel')}</Label>
          <Input
            id={inputId}
            value={tag}
            list={listId}
            placeholder={t('dialog.tagPlaceholder')}
            autoComplete="off"
            onChange={(e) => {
              setTag(e.target.value)
              setDone(null)
            }}
          />
          <datalist id={listId}>
            {(tags ?? []).map((tg) => (
              <option key={tg.id} value={tg.name} />
            ))}
          </datalist>
          <p className="text-xs text-muted-foreground">{t('dialog.tagHint')}</p>
        </div>
        {done ? <Preview result={done} /> : null}
        {!done && previewMatches && previewed ? (
          <Preview result={previewed} />
        ) : null}
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
                disabled={!trimmed || mutation.isPending}
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
                  : t('dialog.apply', {
                      count: previewed?.counts.would_apply ?? 0,
                    })}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
