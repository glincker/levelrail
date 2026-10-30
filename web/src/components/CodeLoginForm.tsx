import { useEffect, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import {
  isMFARequired,
  useFinishSignIn,
  type LoginResult,
} from '../queries/auth'
import { redeemLoginCode, requestLoginCode } from '../queries/signIn'
import { useBrand } from '../hooks/useBrand'
import { cliCommand } from '../lib/cliCommand'
import { LOGIN_CODE_LENGTH, normaliseLoginCode } from '../lib/loginCode'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldDescription, FieldLabel } from './ui/field'
import { Alert, AlertDescription } from './ui/alert'
import { TwoFactorVerifyForm } from './TwoFactorVerifyForm'

type Step = 'request' | 'enter'

const CODE_LENGTH = LOGIN_CODE_LENGTH

function formatRemaining(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60)
  const sec = totalSeconds % 60
  return `${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
}

// Sign in with a one-time code. The server answers the request step the same
// way for every username, so this form never says whether an account exists.
export function CodeLoginForm({
  username,
  onUsernameChange,
  onBack,
}: {
  username: string
  onUsernameChange: (value: string) => void
  onBack: () => void
}) {
  const { t } = useTranslation('signIn')
  const brand = useBrand()
  const finish = useFinishSignIn()
  const [step, setStep] = useState<Step>('request')
  const [code, setCode] = useState('')
  const [mfaToken, setMfaToken] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const request = useMutation({ mutationFn: requestLoginCode })
  const redeem = useMutation<LoginResult, Error, string>({
    mutationFn: redeemLoginCode,
    onSuccess: (result) => {
      if (isMFARequired(result)) {
        setMfaToken(result.mfa_token)
        return
      }
      if ('username' in result) {
        finish(result.username)
      }
    },
  })

  useEffect(() => {
    if (step !== 'enter' || expiresAt === null) {
      return
    }
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [step, expiresAt])

  if (mfaToken) {
    return (
      <TwoFactorVerifyForm
        mfaToken={mfaToken}
        onBack={() => {
          setMfaToken(null)
          setStep('request')
        }}
      />
    )
  }

  const cli = cliCommand(brand.BinaryName, 'auth code')

  if (step === 'request') {
    return (
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (username.trim() === '') {
            return
          }
          request.mutate(username.trim(), {
            onSuccess: (res) => {
              setNow(Date.now())
              setExpiresAt(
                res.expires_in > 0 ? Date.now() + res.expires_in * 1000 : null,
              )
              setStep('enter')
            },
          })
        }}
      >
        <p className="text-sm text-muted-foreground">{t('code.requestHint')}</p>
        <Field>
          <FieldLabel htmlFor="code-login-username">
            {t('code.username')}
          </FieldLabel>
          <Input
            id="code-login-username"
            autoComplete="username"
            value={username}
            onChange={(e) => onUsernameChange(e.target.value)}
          />
        </Field>
        {request.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>{request.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <Button
          type="submit"
          className="w-full"
          disabled={request.isPending || username.trim() === ''}
        >
          {request.isPending ? t('code.sending') : t('code.send')}
        </Button>
        <Button
          type="button"
          variant="link"
          className="w-full text-xs text-muted-foreground"
          onClick={onBack}
        >
          {t('code.usePassword')}
        </Button>
      </form>
    )
  }

  const remaining =
    expiresAt === null
      ? null
      : Math.max(0, Math.floor((expiresAt - now) / 1000))
  const expired = remaining === 0
  const complete = code.length === CODE_LENGTH + 1

  function submit(value: string) {
    if (value.length === CODE_LENGTH + 1 && !redeem.isPending) {
      redeem.mutate(value)
    }
  }

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit(code)
      }}
    >
      {request.data ? (
        <Alert>
          <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
          <AlertDescription>{request.data.message}</AlertDescription>
        </Alert>
      ) : null}
      <Field>
        <FieldLabel htmlFor="code-login-code">{t('code.codeLabel')}</FieldLabel>
        <Input
          id="code-login-code"
          autoComplete="one-time-code"
          inputMode="text"
          autoCapitalize="characters"
          placeholder={t('code.codePlaceholder')}
          autoCorrect="off"
          spellCheck={false}
          aria-describedby="code-login-status"
          className="font-mono text-lg tracking-widest"
          value={code}
          onChange={(e) => {
            const next = normaliseLoginCode(e.target.value)
            setCode(next)
            if (redeem.isError) {
              redeem.reset()
            }
            if (next.length === CODE_LENGTH + 1 && !expired) {
              submit(next)
            }
          }}
        />
        <FieldDescription>{t('code.where', { cli })}</FieldDescription>
        <p
          id="code-login-status"
          role="status"
          aria-live="polite"
          className="text-xs text-muted-foreground tabular-nums"
        >
          {remaining === null
            ? null
            : expired
              ? t('code.expiredHint')
              : t('code.expiresIn', {
                  time: formatRemaining(Math.ceil(remaining / 30) * 30),
                })}
        </p>
      </Field>
      {redeem.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{redeem.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button
        type="submit"
        className="w-full"
        disabled={redeem.isPending || !complete || expired}
      >
        {redeem.isPending ? t('code.verifying') : t('code.verify')}
      </Button>
      <Button
        type="button"
        variant="ghost"
        className="w-full"
        onClick={() => {
          setCode('')
          redeem.reset()
          setStep('request')
        }}
      >
        {t('code.resend')}
      </Button>
      <Button
        type="button"
        variant="link"
        className="w-full text-xs text-muted-foreground"
        onClick={onBack}
      >
        {t('code.usePassword')}
      </Button>
    </form>
  )
}
