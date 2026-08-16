import { CheckCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useForgotPassword } from '../queries/passwordReset'
import { Button } from './ui/button'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'

// No email field: exactly one admin account exists. The response is
// always the same generic success regardless of what actually happened
// server-side, so this form never tries to guess.
export function ForgotPasswordForm() {
  const forgotPassword = useForgotPassword()

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        If a recovery email is on file for the admin account, a password reset
        link will be sent to it.
      </p>

      {forgotPassword.isSuccess ? (
        <Alert>
          <CheckCircleIcon />
          <AlertTitle>Check your email</AlertTitle>
          <AlertDescription>
            If a recovery email is configured, a reset link is on its way.
          </AlertDescription>
        </Alert>
      ) : forgotPassword.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{forgotPassword.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="button"
        className="w-full"
        disabled={forgotPassword.isPending || forgotPassword.isSuccess}
        onClick={() => {
          forgotPassword.mutate()
        }}
      >
        {forgotPassword.isPending ? 'Sending...' : 'Send reset link'}
      </Button>
    </div>
  )
}
