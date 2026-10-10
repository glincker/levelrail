import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CheckCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  isMFARequired,
  useFinishSignIn,
  type LoginResult,
} from '../queries/auth'
import { redeemLoginCode, requestLoginCode } from '../queries/signIn'
import { useBrand } from '../hooks/useBrand'
import { cliCommand } from '../lib/cliCommand'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldDescription, FieldLabel } from './ui/field'
import { Alert, AlertDescription } from './ui/alert'
import { TwoFactorVerifyForm } from './TwoFactorVerifyForm'

type Step = 'request' | 'enter'

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
          request.mutate(username.trim(), { onSuccess: () => setStep('enter') })
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
            <WarningIcon />
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

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (code.trim() !== '') {
          redeem.mutate(code.trim())
        }
      }}
    >
      {request.data ? (
        <Alert>
          <CheckCircleIcon />
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
          className="font-mono tracking-widest"
          value={code}
          onChange={(e) => setCode(e.target.value)}
        />
        <FieldDescription>{t('code.where', { cli })}</FieldDescription>
      </Field>
      {redeem.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{redeem.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button
        type="submit"
        className="w-full"
        disabled={redeem.isPending || code.trim() === ''}
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
