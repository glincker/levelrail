import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CountdownGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { useFinishSignIn } from '../queries/auth'
import { useQuery } from '@tanstack/react-query'
import {
  loginOptionsQueryOptions,
  pollLoginApproval,
  type ApprovalPollStatus,
} from '../queries/signIn'
import { useBrand } from '../hooks/useBrand'
import { cliCommand } from '../lib/cliCommand'
import { Alert, AlertDescription } from './ui/alert'
import { Button } from './ui/button'
import { TwoFactorVerifyForm } from './TwoFactorVerifyForm'

const POLL_MS = 3000

// Shown after a correct password from a new browser: polls until another
// session approves or denies it. The poll is bound to this browser's cookie.
export function NewDeviceApprovalWait({
  approvalId,
  expiresAt,
  matchNumber,
  onBack,
  onUseCode,
}: {
  approvalId: string
  expiresAt: string
  matchNumber: number
  onBack: () => void
  onUseCode: () => void
}) {
  const { t } = useTranslation('signIn')
  const brand = useBrand()
  const finish = useFinishSignIn()
  const options = useQuery(loginOptionsQueryOptions())
  const emailConfigured = options.data?.email_configured ?? true
  const hostCommand = `${brand.BinaryName || 'levelrail'} approve-signin ${approvalId} --match ${matchNumber}`
  const [status, setStatus] = useState<ApprovalPollStatus>('pending')
  const [mfaToken, setMfaToken] = useState<string | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const [totalSeconds] = useState(() =>
    Math.max(
      1,
      Math.floor((new Date(expiresAt).getTime() - Date.now()) / 1000),
    ),
  )

  useEffect(() => {
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [])

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

  const remaining = Math.max(
    0,
    Math.floor((new Date(expiresAt).getTime() - now) / 1000),
  )
  const effective: ApprovalPollStatus =
    status === 'pending' && remaining === 0 ? 'expired' : status
  const announced = Math.ceil(remaining / 30) * 30
  return (
    <div className="space-y-4">
      {effective === 'pending' || effective === 'approved' ? (
        <section
          aria-labelledby="approval-title"
          className="space-y-3 rounded-lg border border-border bg-card p-4 text-center"
        >
          <span className="mx-auto flex size-10 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-500">
            <ShieldCheckGlassIcon
              className="size-6 text-tone-success"
              aria-hidden="true"
            />
          </span>
          <h2 id="approval-title" className="text-sm font-medium">
            {t('approval.waitingTitle')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('approval.waitingHint', {
              cli: cliCommand(brand.BinaryName, 'auth code'),
            })}
          </p>
          {matchNumber > 0 ? (
            <div>
              <p className="text-xs text-muted-foreground">
                {t('approval.matchLabel')}
              </p>
              <p
                className="font-mono text-4xl font-semibold tracking-widest text-foreground tabular-nums"
                data-testid="approval-match"
                aria-label={t('approval.matchAria', { number: matchNumber })}
              >
                {matchNumber}
              </p>
            </div>
          ) : null}
          <p className="flex items-center justify-center gap-1.5 text-sm text-muted-foreground">
            <CountdownGlassIcon className="size-5" aria-hidden="true" />
            <span aria-hidden="true" data-testid="approval-countdown">
              {t('approval.countdown', { time: formatRemaining(remaining) })}
            </span>
          </p>
          <progress
            aria-hidden="true"
            value={remaining}
            max={totalSeconds}
            className="mx-auto block h-1 w-full max-w-48 overflow-hidden rounded-full [&::-moz-progress-bar]:bg-emerald-500 [&::-webkit-progress-bar]:bg-muted [&::-webkit-progress-value]:bg-emerald-500"
          />
          <p className="sr-only" role="status" aria-live="polite">
            {t('approval.countdown', { time: formatRemaining(announced) })}
          </p>
          <p className="font-mono text-xs text-muted-foreground">
            {t('approval.reference', { id: approvalId })}
          </p>
        </section>
      ) : (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>
            {effective === 'denied'
              ? t('approval.denied')
              : t('approval.expired')}
          </AlertDescription>
        </Alert>
      )}
      {effective === 'pending' ? (
        <details className="rounded-lg border border-border bg-card p-3 text-left">
          <summary className="cursor-pointer text-sm font-medium text-foreground">
            {t('approval.hostTitle')}
          </summary>
          <p className="mt-2 text-xs text-muted-foreground">
            {t('approval.hostHint')}
          </p>
          <code className="mt-2 block overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs text-foreground">
            {hostCommand}
          </code>
        </details>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {emailConfigured
          ? t('approval.alternative')
          : t('approval.alternativeNoEmail')}
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

function formatRemaining(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60)
  const sec = totalSeconds % 60
  return `${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
}
