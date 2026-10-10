import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { HourglassIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useFinishSignIn } from '../queries/auth'
import { pollLoginApproval, type ApprovalPollStatus } from '../queries/signIn'
import { useBrand } from '../hooks/useBrand'
import { cliCommand } from '../lib/cliCommand'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'
import { Button } from './ui/button'
import { TwoFactorVerifyForm } from './TwoFactorVerifyForm'

const POLL_MS = 3000

// Shown after a correct password from a new browser: polls until another
// session approves or denies it. The poll is bound to this browser's cookie.
export function NewDeviceApprovalWait({
  approvalId,
  expiresAt,
  onBack,
  onUseCode,
}: {
  approvalId: string
  expiresAt: string
  onBack: () => void
  onUseCode: () => void
}) {
  const { t } = useTranslation('signIn')
  const brand = useBrand()
  const finish = useFinishSignIn()
  const [status, setStatus] = useState<ApprovalPollStatus>('pending')
  const [mfaToken, setMfaToken] = useState<string | null>(null)

  useEffect(() => {
    if (status !== 'pending') {
      return
    }
    const id = window.setInterval(() => {
      pollLoginApproval()
        .then((res) => {
          if (res.status === 'approved' && res.mfa_required && res.mfa_token) {
            setMfaToken(res.mfa_token)
          } else if (res.status === 'approved' && res.username) {
            finish(res.username)
          }
          setStatus(res.status)
        })
        .catch(() => {
          setStatus('expired')
        })
    }, POLL_MS)
    return () => {
      window.clearInterval(id)
    }
  }, [status, finish])

  if (mfaToken) {
    return <TwoFactorVerifyForm mfaToken={mfaToken} onBack={onBack} />
  }

  const until = new Date(expiresAt).toLocaleTimeString()
  return (
    <div className="space-y-4">
      {status === 'pending' || status === 'approved' ? (
        <Alert>
          <HourglassIcon />
          <AlertTitle>{t('approval.waitingTitle')}</AlertTitle>
          <AlertDescription>
            <p>
              {t('approval.waitingHint', {
                cli: cliCommand(brand.BinaryName, 'auth code'),
              })}
            </p>
            <p className="font-mono text-xs">
              {t('approval.reference', { id: approvalId })}
            </p>
            <p>{t('approval.expires', { time: until })}</p>
          </AlertDescription>
        </Alert>
      ) : (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>
            {status === 'denied' ? t('approval.denied') : t('approval.expired')}
          </AlertDescription>
        </Alert>
      )}
      <p className="text-xs text-muted-foreground">
        {t('approval.alternative')}
      </p>
      <Button
        type="button"
        variant="outline"
        className="w-full"
        onClick={onUseCode}
      >
        {t('code.useCode')}
      </Button>
      <Button
        type="button"
        variant="link"
        className="w-full text-xs text-muted-foreground"
        onClick={onBack}
      >
        {t('approval.back')}
      </Button>
    </div>
  )
}
