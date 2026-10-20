import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useAcceptInvite } from '../queries/invites'
import { Button } from './ui/button'
import { Input } from './ui/input'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from './ui/field'
import { Alert, AlertDescription } from './ui/alert'

// Mirrors internal/api's minPasswordLength (8), the same client-side
// head start ResetPasswordForm gives.
const MIN_PASSWORD_LENGTH = 8

const acceptInviteSchema = z
  .object({
    password: z
      .string()
      .min(
        MIN_PASSWORD_LENGTH,
        `Password must be at least ${MIN_PASSWORD_LENGTH} characters`,
      ),
    confirmPassword: z.string(),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

type AcceptInviteValues = z.infer<typeof acceptInviteSchema>

// token comes from the invite link's query string (routes/accept-invite.tsx),
// never typed by hand. On success the caller is signed straight in
// (useAcceptInvite navigates to "/"), matching RegisterForm's own
// self-service account creation flow.
export function AcceptInviteForm({ token }: { token: string }) {
  const acceptInvite = useAcceptInvite()
  const { register, handleSubmit, formState } = useForm<AcceptInviteValues>({
    resolver: zodResolver(acceptInviteSchema),
    defaultValues: { password: '', confirmPassword: '' },
  })

  const onSubmit = handleSubmit((values) => {
    acceptInvite.mutate({ token, password: values.password })
  })

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="space-y-4"
    >
      <FieldGroup>
        <Field data-invalid={formState.errors.password ? true : undefined}>
          <FieldLabel htmlFor="invite-password">Password</FieldLabel>
          <Input
            id="invite-password"
            type="password"
            autoComplete="new-password"
            aria-invalid={!!formState.errors.password}
            {...register('password')}
          />
          {formState.errors.password ? (
            <FieldError errors={[formState.errors.password]} />
          ) : (
            <FieldDescription>
              At least {MIN_PASSWORD_LENGTH} characters.
            </FieldDescription>
          )}
        </Field>
        <Field
          data-invalid={formState.errors.confirmPassword ? true : undefined}
        >
          <FieldLabel htmlFor="invite-confirm-password">
            Confirm password
          </FieldLabel>
          <Input
            id="invite-confirm-password"
            type="password"
            autoComplete="new-password"
            aria-invalid={!!formState.errors.confirmPassword}
            {...register('confirmPassword')}
          />
          <FieldError errors={[formState.errors.confirmPassword]} />
        </Field>
      </FieldGroup>

      {acceptInvite.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{acceptInvite.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="submit"
        className="w-full"
        disabled={acceptInvite.isPending}
      >
        {acceptInvite.isPending ? 'Creating account...' : 'Accept invite'}
      </Button>
    </form>
  )
}
