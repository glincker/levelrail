import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import { useEffect, useMemo, useState } from 'react'
import {
  ArrowLeftIcon,
  ClockIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { RateLimitError, useVerifyTwoFactor } from '../queries/auth'
import { loginOptionsQueryOptions } from '../queries/signIn'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Checkbox } from './ui/checkbox'
import { Field, FieldError, FieldGroup, FieldLabel } from './ui/field'
import { Alert, AlertDescription } from './ui/alert'

interface TwoFactorVerifyFormProps {
  mfaToken: string
  onBack: () => void
}

// Step two of login for an account with TOTP on. The browser is remembered
// only when the box is ticked, never by default.
export function TwoFactorVerifyForm({
  mfaToken,
  onBack,
}: TwoFactorVerifyFormProps) {
  const { t } = useTranslation('signIn')
  const verify = useVerifyTwoFactor()
  const options = useQuery(loginOptionsQueryOptions())
  const [useRecoveryCode, setUseRecoveryCode] = useState(false)
  const [remember, setRemember] = useState(false)
  const codeSchema = useMemo(
    () =>
      z.object({ code: z.string().trim().min(1, t('twoFactor.codeRequired')) }),
    [t],
  )
  const { register, handleSubmit, formState, reset } = useForm<{
    code: string
  }>({
    resolver: zodResolver(codeSchema),
    defaultValues: { code: '' },
  })

  const [secondsRemaining, setSecondsRemaining] = useState(0)
  const isRateLimited = secondsRemaining > 0

  useEffect(() => {
    if (!isRateLimited) {
      return
    }
    const id = window.setInterval(() => {
      setSecondsRemaining((s) => Math.max(0, s - 1))
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [isRateLimited])

  const onSubmit = handleSubmit((values) => {
    verify.mutate(
      useRecoveryCode
        ? { mfaToken, recoveryCode: values.code, rememberDevice: remember }
        : { mfaToken, code: values.code, rememberDevice: remember },
      {
        onError: (error) => {
          if (error instanceof RateLimitError) {
            setSecondsRemaining(error.retryAfterSeconds)
          }
        },
      },
    )
  })

  function toggleMode() {
    setUseRecoveryCode((v) => !v)
    reset({ code: '' })
    verify.reset()
  }

  const days = options.data?.trusted_device_days ?? 0

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="mt-4 space-y-4"
    >
      <FieldGroup>
        <Field data-invalid={formState.errors.code ? true : undefined}>
          <FieldLabel htmlFor="mfa-code">
            {useRecoveryCode
              ? t('twoFactor.recoveryLabel')
              : t('twoFactor.codeLabel')}
          </FieldLabel>
          <Input
            id="mfa-code"
            autoComplete="one-time-code"
            autoFocus
            placeholder={
              useRecoveryCode
                ? t('twoFactor.recoveryPlaceholder')
                : t('twoFactor.codePlaceholder')
            }
            aria-invalid={!!formState.errors.code}
            {...register('code')}
          />
          <FieldError errors={[formState.errors.code]} />
        </Field>
        {days > 0 ? (
          <Field orientation="horizontal">
            <Checkbox
              id="mfa-remember"
              checked={remember}
              onCheckedChange={(checked) => {
                setRemember(checked === true)
              }}
            />
            <FieldLabel htmlFor="mfa-remember" className="font-normal">
              {t('twoFactor.remember', { count: days })}
            </FieldLabel>
          </Field>
        ) : null}
      </FieldGroup>

      {isRateLimited ? (
        <Alert variant="destructive">
          <ClockIcon />
          <AlertDescription>
            {t('twoFactor.rateLimited', { seconds: secondsRemaining })}
          </AlertDescription>
        </Alert>
      ) : verify.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{verify.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="submit"
        className="w-full"
        disabled={verify.isPending || isRateLimited}
      >
        {verify.isPending ? t('twoFactor.verifying') : t('twoFactor.verify')}
      </Button>

      <div className="flex items-center justify-between text-xs">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1 text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3" />
          {t('twoFactor.back')}
        </button>
        <button
          type="button"
          onClick={toggleMode}
          className="text-muted-foreground hover:text-foreground"
        >
          {useRecoveryCode
            ? t('twoFactor.useAuthenticator')
            : t('twoFactor.useRecovery')}
        </button>
      </div>
    </form>
  )
}
