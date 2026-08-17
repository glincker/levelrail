import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useEffect, useState } from 'react'
import {
  ArrowLeftIcon,
  ClockIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { RateLimitError, useVerifyTwoFactor } from '../queries/auth'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldError, FieldGroup, FieldLabel } from './ui/field'
import { Alert, AlertDescription } from './ui/alert'

const codeSchema = z.object({
  code: z.string().trim().min(1, 'Enter a code'),
})

type CodeFormValues = z.infer<typeof codeSchema>

interface TwoFactorVerifyFormProps {
  mfaToken: string
  onBack: () => void
}

// Step two of login for an account with TOTP enabled
// (internal/api/twofactor.go's handleVerifyTwoFactor), shown by
// LoginForm once handleLogin's own response carries mfa_required. Same
// RateLimitError countdown shape LoginForm uses for password attempts,
// this endpoint has its own independent rate limit
// (Router.mfaVerify) so the same UI treatment applies unchanged.
export function TwoFactorVerifyForm({
  mfaToken,
  onBack,
}: TwoFactorVerifyFormProps) {
  const verify = useVerifyTwoFactor()
  const [useRecoveryCode, setUseRecoveryCode] = useState(false)
  const { register, handleSubmit, formState, reset } = useForm<CodeFormValues>({
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
        ? { mfaToken, recoveryCode: values.code }
        : { mfaToken, code: values.code },
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
            {useRecoveryCode ? 'Recovery code' : 'Authenticator code'}
          </FieldLabel>
          <Input
            id="mfa-code"
            autoComplete="one-time-code"
            autoFocus
            placeholder={useRecoveryCode ? 'xxxx-xxxx-xxxx-xxxx' : '123456'}
            aria-invalid={!!formState.errors.code}
            {...register('code')}
          />
          <FieldError errors={[formState.errors.code]} />
        </Field>
      </FieldGroup>

      {isRateLimited ? (
        <Alert variant="destructive">
          <ClockIcon />
          <AlertDescription>
            Too many attempts. Try again in {secondsRemaining}s.
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
        {verify.isPending ? 'Verifying...' : 'Verify'}
      </Button>

      <div className="flex items-center justify-between text-xs">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1 text-muted-foreground hover:text-foreground"
        >
          <ArrowLeftIcon className="size-3" />
          Back to sign in
        </button>
        <button
          type="button"
          onClick={toggleMode}
          className="text-muted-foreground hover:text-foreground"
        >
          {useRecoveryCode ? 'Use authenticator code' : 'Use a recovery code'}
        </button>
      </div>
    </form>
  )
}
