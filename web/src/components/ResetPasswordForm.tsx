import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { CheckCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useResetPassword } from '../queries/passwordReset'
import { Button } from './ui/button'
import { Input } from './ui/input'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from './ui/field'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'

// Mirrors internal/api/account.go's minPasswordLength (8), the same
// client-side head start ChangePasswordCard/RegisterForm already give.
const MIN_PASSWORD_LENGTH = 8

const resetPasswordSchema = z
  .object({
    newPassword: z
      .string()
      .min(
        MIN_PASSWORD_LENGTH,
        `Password must be at least ${MIN_PASSWORD_LENGTH} characters`,
      ),
    confirmPassword: z.string(),
  })
  .refine((data) => data.newPassword === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

type ResetPasswordValues = z.infer<typeof resetPasswordSchema>

// token comes from the reset link's query string (routes/reset-password.tsx),
// never typed by hand.
export function ResetPasswordForm({ token }: { token: string }) {
  const navigate = useNavigate()
  const resetPassword = useResetPassword()
  const { register, handleSubmit, formState } = useForm<ResetPasswordValues>({
    resolver: zodResolver(resetPasswordSchema),
    defaultValues: { newPassword: '', confirmPassword: '' },
  })

  const onSubmit = handleSubmit((values) => {
    resetPassword.mutate({ token, newPassword: values.newPassword })
  })

  if (resetPassword.isSuccess) {
    return (
      <div className="space-y-4">
        <Alert>
          <CheckCircleIcon />
          <AlertTitle>Password reset</AlertTitle>
          <AlertDescription>
            Your password has been changed. Every other session was signed out.
          </AlertDescription>
        </Alert>
        <Button
          type="button"
          className="w-full"
          onClick={() => {
            void navigate({ to: '/login' })
          }}
        >
          Go to sign in
        </Button>
      </div>
    )
  }

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-4"
    >
      <FieldGroup>
        <Field data-invalid={formState.errors.newPassword ? true : undefined}>
          <FieldLabel htmlFor="reset-new-password">New password</FieldLabel>
          <Input
            id="reset-new-password"
            type="password"
            autoComplete="new-password"
            aria-invalid={!!formState.errors.newPassword}
            {...register('newPassword')}
          />
          {formState.errors.newPassword ? (
            <FieldError errors={[formState.errors.newPassword]} />
          ) : (
            <FieldDescription>
              At least {MIN_PASSWORD_LENGTH} characters.
            </FieldDescription>
          )}
        </Field>
        <Field
          data-invalid={formState.errors.confirmPassword ? true : undefined}
        >
          <FieldLabel htmlFor="reset-confirm-password">
            Confirm new password
          </FieldLabel>
          <Input
            id="reset-confirm-password"
            type="password"
            autoComplete="new-password"
            aria-invalid={!!formState.errors.confirmPassword}
            {...register('confirmPassword')}
          />
          <FieldError errors={[formState.errors.confirmPassword]} />
        </Field>
      </FieldGroup>

      {resetPassword.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{resetPassword.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="submit"
        className="w-full"
        disabled={resetPassword.isPending}
      >
        {resetPassword.isPending ? 'Resetting...' : 'Reset password'}
      </Button>
    </form>
  )
}
