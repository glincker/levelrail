import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
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
import { formatLeft, useCliPairing } from '../../hooks/useCliPairing'
import { myCredentialsKey } from '../../queries/myCredentials'
import { useRevokeToken } from '../../queries/tokens'
import type { TokenResource } from '../../types/token'

type Step = 'confirm' | 'revoking' | 'revoked'

// Revokes one CLI token, then offers a fresh one-time pairing command. The
// old token is never shown and nothing rotates without this confirmation.
export function RotateCliTokenDialog({
  token,
  onClose,
}: {
  token: TokenResource
  onClose: () => void
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const qc = useQueryClient()
  const revoke = useRevokeToken()
  const p = useCliPairing()
  const [step, setStep] = useState<Step>('confirm')
  const [error, setError] = useState(false)

  const rotate = () => {
    setError(false)
    setStep('revoking')
    revoke.mutate(token.id, {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: myCredentialsKey })
        setStep('revoked')
        p.generate()
      },
      onError: () => {
        setError(true)
        setStep('confirm')
      },
    })
  }

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next && step !== 'revoking') {
          onClose()
        }
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {step === 'revoked'
              ? t('cliModal.manage.rotateDoneTitle')
              : t('cliModal.manage.rotateTitle')}
          </DialogTitle>
          <DialogDescription>
            {step === 'revoked'
              ? t('cliModal.manage.rotateDoneBody')
              : t('cliModal.manage.rotateBody', { name: token.name })}
          </DialogDescription>
        </DialogHeader>
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t('cliModal.manage.rotateFailed')}
            </AlertDescription>
          </Alert>
        ) : null}
        {step === 'revoked' ? (
          <div aria-live="polite" className="space-y-2">
            {p.live ? (
              <>
                <code
                  aria-label={t('connectCli.commandLabel')}
                  data-testid="rotate-cli-command"
                  className="block select-all overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs"
                >
                  {p.command}
                </code>
                <p className="text-xs text-muted-foreground">
                  {t('connectCli.expiresIn', { time: formatLeft(p.left) })}
                </p>
              </>
            ) : p.failed ? (
              <p role="alert" className="text-sm text-muted-foreground">
                {t('cliModal.manage.rotatePairFailed')}
              </p>
            ) : p.expired ? (
              <p className="text-sm text-muted-foreground">
                {t('connectCli.expired')}
              </p>
            ) : (
              <p className="text-sm text-muted-foreground">
                {t('cliModal.manage.rotatePairing')}
              </p>
            )}
          </div>
        ) : null}
        <DialogFooter>
          {step === 'revoked' ? (
            <>
              {p.live ? (
                <Button type="button" onClick={p.copy}>
                  {p.copied ? (
                    <CheckGlassIcon size="1.4em" aria-hidden="true" />
                  ) : (
                    <CopyGlassIcon size="1.4em" aria-hidden="true" />
                  )}
                  {t('connectCli.copy')}
                </Button>
              ) : (
                <Button type="button" disabled={p.pending} onClick={p.generate}>
                  {t('cliModal.manage.rotateRetry')}
                </Button>
              )}
              <Button type="button" variant="outline" onClick={onClose}>
                {t('cliModal.manage.rotateClose')}
              </Button>
            </>
          ) : (
            <>
              <Button
                type="button"
                variant="outline"
                disabled={step === 'revoking'}
                onClick={onClose}
              >
                {t('cliModal.manage.rotateCancel')}
              </Button>
              <Button
                type="button"
                variant="destructive"
                writeAction
                disabled={step === 'revoking'}
                onClick={rotate}
              >
                {step === 'revoking'
                  ? t('cliModal.manage.rotating')
                  : t('cliModal.manage.rotateConfirm')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
