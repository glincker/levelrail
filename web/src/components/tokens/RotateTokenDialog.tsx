import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CopyGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useBrand } from '../../hooks/useBrand'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { ApiError } from '../../lib/apiError'
import { useRotateToken } from '../../queries/tokens'
import type { RotateTokenResponse, TokenResource } from '../../types/token'

// The secret lives only in this state: the mutation result is dropped at
// once and unmounting on close discards it.
export function RotateTokenDialog({
  token,
  grace,
  onClose,
}: {
  token: TokenResource
  grace: string | null
  onClose: () => void
}) {
  const { t } = useTranslation('tokenExpiry')
  const brand = useBrand()
  const rotate = useRotateToken()
  const { copied, copy } = useCopyToClipboard()
  const [result, setResult] = useState<RotateTokenResponse | null>(null)
  const [failure, setFailure] = useState<Error | null>(null)
  const validUntil = result
    ? (result.previous_valid_until ?? result.old_token_valid_until)
    : null
  const cli = `${brand.BinaryName || 'levelrail-cli'} tokens rotate ${token.name}`

  const close = () => {
    setResult(null)
    rotate.reset()
    onClose()
  }

  const confirm = () => {
    setFailure(null)
    rotate.mutate(token.id, {
      onSuccess: (res) => {
        setResult(res)
        rotate.reset()
      },
      onError: setFailure,
    })
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next && !rotate.isPending) close()
      }}
    >
      <DialogContent className="sm:max-w-md">
        {result ? (
          <>
            <DialogHeader>
              <DialogTitle>
                {t('rotate.doneTitle', { name: token.name })}
              </DialogTitle>
              <DialogDescription>{t('rotate.doneBody')}</DialogDescription>
            </DialogHeader>
            <Alert variant="warning" className="flex items-start gap-2">
              <TriangleAlertGlassIcon
                className="mt-0.5 size-6 shrink-0 text-tone-warning"
                aria-hidden="true"
              />
              <AlertDescription>{t('rotate.warning')}</AlertDescription>
            </Alert>
            <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
              <code
                aria-label={t('rotate.secretLabel')}
                className="min-w-0 flex-1 overflow-x-auto text-xs break-all select-all"
              >
                {result.token}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  copy(result.token)
                }}
              >
                {copied ? (
                  <CheckGlassIcon size="1.4em" aria-hidden="true" />
                ) : (
                  <CopyGlassIcon size="1.4em" aria-hidden="true" />
                )}
                {copied ? t('rotate.copied') : t('rotate.copy')}
              </Button>
            </div>
            <p className="text-sm text-muted-foreground" aria-live="polite">
              {validUntil
                ? t('rotate.oldUntil', {
                    time: new Date(validUntil).toLocaleString(),
                  })
                : t('rotate.oldRevoked')}
            </p>
            <DialogFooter>
              <Button type="button" onClick={close}>
                {t('rotate.done')}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>
                {t('rotate.title', { name: token.name })}
              </DialogTitle>
              <DialogDescription>{t('rotate.body')}</DialogDescription>
            </DialogHeader>
            <p className="text-sm text-muted-foreground">
              {grace ? t('rotate.grace', { grace }) : t('rotate.noGrace')}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('rotate.cli')}{' '}
              <code className="rounded bg-muted px-1.5 py-0.5 font-mono break-all">
                {cli}
              </code>
            </p>
            {failure ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {t(errorKey(failure), { message: failure.message })}
                </AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={rotate.isPending}
                onClick={close}
              >
                {t('rotate.cancel')}
              </Button>
              <Button
                type="button"
                writeAction
                disabled={rotate.isPending}
                onClick={confirm}
              >
                {rotate.isPending ? t('rotate.pending') : t('rotate.confirm')}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function errorKey(err: Error) {
  if (err instanceof ApiError) {
    if (err.status === 403) return 'rotate.errors.forbidden'
    if (err.status === 409) return 'rotate.errors.conflict'
    if (err.status === 404) return 'rotate.errors.notFound'
    return 'rotate.errors.generic'
  }
  return 'rotate.errors.network'
}
