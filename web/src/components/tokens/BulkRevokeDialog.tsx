import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
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
import { RelativeTime } from '../kit/RelativeTime'
import {
  tokenCountsQueryOptions,
  tokenRecentQueryOptions,
  useBulkRevoke,
} from '../../queries/tokenList'
import {
  TOKEN_RECENT_USE_MS,
  TOKEN_TYPED_CONFIRM_ABOVE,
} from '../../lib/tokenListConfig'
import type { TokenSelection } from '../../lib/tokenSelection'
import type { BulkRevokeResult, ServedToken } from '../../types/tokenList'

interface BulkRevokeDialogProps {
  selection: TokenSelection
  // Tokens the lists have loaded, used to describe an explicit id selection.
  known: ReadonlyMap<string, ServedToken>
  onClose: (result: { revoked: number; skipped: number } | null) => void
}

function danger(tokens: readonly ServedToken[], now: number) {
  const live = tokens.filter((tok) => tok.status !== 'revoked')
  const recent = live.filter(
    (tok) =>
      tok.last_used_at &&
      now - Date.parse(tok.last_used_at) < TOKEN_RECENT_USE_MS,
  )
  const latest = recent.reduce<string | undefined>(
    (acc, tok) =>
      !acc || (tok.last_used_at ?? '') > acc ? tok.last_used_at : acc,
    undefined,
  )
  return {
    root: live.filter((tok) => tok.abilities.includes('root')).length,
    recent: recent.length,
    latest,
  }
}

export function BulkRevokeDialog({
  selection,
  known,
  onClose,
}: BulkRevokeDialogProps) {
  const { t } = useTranslation('tokenList')
  const bulk = useBulkRevoke()
  const [preview, setPreview] = useState<BulkRevokeResult | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [result, setResult] = useState<BulkRevokeResult | null>(null)
  const [typed, setTyped] = useState('')
  const [now] = useState(() => Date.now())
  const typedId = useId()
  const cancelRef = useRef<HTMLButtonElement>(null)

  const request = useMemo(
    () =>
      selection.mode === 'ids'
        ? { ids: [...selection.ids] }
        : { filter: selection.filter },
    [selection],
  )

  // Always dry run first: the server, not the page, says what would be revoked.
  const dryRun = bulk.mutateAsync
  useEffect(() => {
    let live = true
    dryRun({ ...request, dry_run: true })
      .then((res) => {
        if (live) setPreview(res)
      })
      .catch((err: unknown) => {
        if (live) setPreviewError(err instanceof Error ? err.message : '')
      })
    return () => {
      live = false
    }
  }, [dryRun, request])

  const filterMode = selection.mode === 'filter'
  const filterArg = selection.mode === 'filter' ? selection.filter : {}
  const rootCounts = useQuery({
    ...tokenCountsQueryOptions({ ...filterArg, ability: 'root' }),
    enabled: filterMode,
  })
  const recent = useQuery({
    ...tokenRecentQueryOptions(filterArg),
    enabled: filterMode,
  })
  const summary = useMemo(() => {
    if (selection.mode === 'ids') {
      const picked = [...selection.ids].flatMap((id) => {
        const tok = known.get(id)
        return tok ? [tok] : []
      })
      return danger(picked, now)
    }
    const d = danger(recent.data ?? [], now)
    const rc = rootCounts.data
    return { ...d, root: rc ? rc.total - rc.counts.revoked : 0 }
  }, [selection, known, now, recent.data, rootCounts.data])

  const count = preview?.count ?? 0
  const needsTyping = count > TOKEN_TYPED_CONFIRM_ABOVE
  const typedOk = !needsTyping || typed.trim() === String(count)
  const revoking = bulk.isPending && preview !== null

  function confirm() {
    if (!preview) return
    bulk.mutate(
      { ...request, confirm_count: preview.count },
      {
        onSuccess: (res) => {
          setResult(res)
        },
      },
    )
  }

  const error =
    previewError ?? (bulk.isError && preview ? bulk.error.message : null)
  const requested =
    selection.mode === 'ids' ? selection.ids.size : (preview?.count ?? 0)

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (open) return
        onClose(
          result
            ? {
                revoked: result.revoked,
                skipped: Math.max(0, requested - result.revoked),
              }
            : null,
        )
      }}
    >
      <DialogContent size="confirm" initialFocus={cancelRef}>
        {result ? (
          <>
            <DialogHeader>
              <DialogTitle>{t('revoke.resultTitle')}</DialogTitle>
              <DialogDescription role="status" aria-live="polite">
                {t('revoke.result', {
                  revoked: result.revoked,
                  skipped: Math.max(0, requested - result.revoked),
                })}
              </DialogDescription>
            </DialogHeader>
            <p className="text-sm text-muted-foreground">
              {t('revoke.resultNote')}
            </p>
            <DialogFooter>
              <Button
                ref={cancelRef}
                onClick={() => {
                  onClose({
                    revoked: result.revoked,
                    skipped: Math.max(0, requested - result.revoked),
                  })
                }}
              >
                {t('revoke.close')}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>
                {t('revoke.title', { count: preview ? count : requested })}
              </DialogTitle>
              <DialogDescription aria-live="polite">
                {preview
                  ? t('revoke.willRevoke', { count })
                  : t('revoke.checking')}
              </DialogDescription>
            </DialogHeader>
            {preview && preview.sample.length > 0 ? (
              <p className="text-sm break-words text-muted-foreground">
                {t('revoke.sample', { names: preview.sample.join(', ') })}
                {count > preview.sample.length
                  ? ` ${t('revoke.sampleMore', { count: count - preview.sample.length })}`
                  : ''}
              </p>
            ) : null}
            {preview && count === 0 ? (
              <p className="text-sm">{t('revoke.none')}</p>
            ) : null}
            <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
              <li>{t('revoke.permanent')}</li>
              {summary.root > 0 ? (
                <li className="font-medium text-tone-danger">
                  {t('revoke.rootIncluded', { count: summary.root })}
                </li>
              ) : null}
              {summary.recent > 0 && summary.latest ? (
                <li className="font-medium text-tone-warning">
                  {t('revoke.recentIncluded', { count: summary.recent })}{' '}
                  <RelativeTime at={summary.latest} />
                </li>
              ) : null}
              <li>{t('revoke.selfExcluded')}</li>
            </ul>
            {needsTyping ? (
              <div className="space-y-1.5">
                <Label htmlFor={typedId}>
                  {t('revoke.typePrompt', { count })}
                </Label>
                <Input
                  id={typedId}
                  inputMode="numeric"
                  autoComplete="off"
                  aria-label={t('revoke.typeLabel')}
                  value={typed}
                  onChange={(e) => {
                    setTyped(e.target.value)
                  }}
                />
              </div>
            ) : null}
            {error ? (
              <p role="alert" className="text-sm text-tone-danger">
                {t('revoke.failed', { message: error })}
              </p>
            ) : null}
            <DialogFooter>
              <Button
                ref={cancelRef}
                variant="outline"
                onClick={() => {
                  onClose(null)
                }}
              >
                {t('bar.clear')}
              </Button>
              <Button
                variant="destructive-solid"
                disabled={!preview || count === 0 || !typedOk || revoking}
                onClick={confirm}
              >
                {revoking
                  ? t('revoke.pending')
                  : t('revoke.confirm', { count })}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
