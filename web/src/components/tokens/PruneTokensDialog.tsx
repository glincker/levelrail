import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BroomGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { usePruneTokens } from '../../queries/tokenList'
import { TOKEN_PRUNE_DAYS } from '../../lib/tokenListConfig'
import type { PruneResult } from '../../types/tokenList'

// Root only; the server stays the authority and answers 403 for anyone else.
export function PruneTokensDialog({ disabled }: { disabled: boolean }) {
  const { t } = useTranslation('tokenList')
  const [open, setOpen] = useState(false)
  const [days, setDays] = useState(String(TOKEN_PRUNE_DAYS))
  const [preview, setPreview] = useState<PruneResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const prune = usePruneTokens()
  const run = prune.mutateAsync
  const id = useId()
  const n = Number(days)
  const valid = days !== '' && Number.isInteger(n) && n >= 0

  // Dry run on open and whenever the window changes, so the count is never stale.
  useEffect(() => {
    if (!open || !valid) return undefined
    let live = true
    const timer = window.setTimeout(() => {
      run({ days: n, dryRun: true })
        .then((res) => {
          if (live) {
            setPreview(res)
            setError(null)
          }
        })
        .catch((err: unknown) => {
          if (live) setError(err instanceof Error ? err.message : '')
        })
    }, 200)
    return () => {
      live = false
      window.clearTimeout(timer)
    }
  }, [open, valid, n, run])

  const total = preview ? preview.revoked + preview.expired : 0

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) setPreview(null)
      }}
    >
      <DialogTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            title={disabled ? t('prune.rootOnly') : undefined}
          />
        }
      >
        <BroomGlassIcon aria-hidden="true" />
        {t('prune.trigger')}
      </DialogTrigger>
      <DialogContent size="confirm">
        <DialogHeader>
          <DialogTitle>{t('prune.title')}</DialogTitle>
          <DialogDescription>{t('prune.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor={id}>{t('prune.days')}</Label>
          <Input
            id={id}
            type="number"
            min={0}
            value={days}
            onChange={(e) => {
              setDays(e.target.value)
              setPreview(null)
            }}
          />
        </div>
        <p role="status" aria-live="polite" className="text-sm">
          {preview
            ? total > 0
              ? t('prune.preview', {
                  revoked: preview.revoked,
                  expired: preview.expired,
                })
              : t('prune.nothing')
            : t('prune.checking')}
        </p>
        {error ? (
          <p role="alert" className="text-sm text-tone-danger">
            {t('prune.failed', { message: error })}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            variant="destructive-solid"
            disabled={!preview || total === 0 || prune.isPending}
            onClick={() => {
              run({ days: n, dryRun: false })
                .then(() => {
                  setOpen(false)
                  setPreview(null)
                })
                .catch((err: unknown) => {
                  setError(err instanceof Error ? err.message : '')
                })
            }}
          >
            {prune.isPending
              ? t('prune.pending')
              : t('prune.confirm', { count: total })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
