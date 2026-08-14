import { createFileRoute } from '@tanstack/react-router'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  CheckCircleIcon,
  WarningIcon,
  UserIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { useChangePassword } from '../../queries/account'
import { Button } from '../../components/ui/button'
import { Input } from '../../components/ui/input'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '../../components/ui/field'
import { Alert, AlertDescription, AlertTitle } from '../../components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../components/ui/card'

// Real account page, following on from the placeholder shell this route
// used to be (see git history / the previous version of this file for
// why it was split out on its own). Two independent cards: profile
// (read-only, Phase 1 has exactly one editable-nothing admin account:
// single admin user, session auth, no teams or RBAC yet) and
// change-password (a real form against internal/api/account.go's PUT
// /api/v1/auth/password).
export const Route = createFileRoute('/settings/account')({
  component: AccountSettingsPage,
})

function AccountSettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Account</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Profile and password.
        </p>
      </div>

      <ProfileCard />
      <ChangePasswordCard />
    </div>
  )
}

// store.AdminUser (internal/store/admin.go) has exactly two fields:
// Username and PasswordHash. No email, display name, or avatar exist on
// the backend to show or edit here, so this card is deliberately just a
// read-only username, not a stub for fields that don't exist yet.
function ProfileCard() {
  const username = useAuthUsername()

  return (
    <Card>
      <CardHeader>
        <CardTitle>Profile</CardTitle>
        <CardDescription>
          Levelrail is single-admin in this phase: one account, no teams, no
          roles yet.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Field>
          <FieldLabel htmlFor="account-username">Username</FieldLabel>
          <div
            id="account-username"
            className="flex h-9 items-center gap-2 rounded-md border border-border bg-muted/50 px-3 text-sm text-foreground"
          >
            <UserIcon className="size-4 text-muted-foreground" />
            {username ?? 'Unknown'}
          </div>
          <FieldDescription>
            There is no other editable profile field yet in this phase, no
            email, display name, or avatar.
          </FieldDescription>
        </Field>
      </CardContent>
    </Card>
  )
}

// Mirrors internal/api/account.go's minPasswordLength (8) exactly, the
// same client-side head start RegisterForm already established for the
// same rule: a fast local check for a quicker feedback loop, never a
// substitute for the server's own check, which still runs and still
// wins if the two ever drift.
const MIN_PASSWORD_LENGTH = 8

const changePasswordSchema = z
  .object({
    currentPassword: z.string().min(1, 'Current password is required'),
    newPassword: z
      .string()
      .min(
        MIN_PASSWORD_LENGTH,
        `New password must be at least ${MIN_PASSWORD_LENGTH} characters`,
      ),
    confirmPassword: z.string(),
  })
  .refine((data) => data.newPassword === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

type ChangePasswordValues = z.infer<typeof changePasswordSchema>

// internal/api/account.go's handleChangePassword collapses "no session"
// and "wrong current password" into the same 401 with the same message,
// on purpose (its own doc comment cites the password-reconfirmation
// finding this mirrors), so this form never tries to guess which one
// happened, it just surfaces the server's message as-is on both 401 and
// 400.
function ChangePasswordCard() {
  const changePassword = useChangePassword()
  const { register, handleSubmit, formState, reset } =
    useForm<ChangePasswordValues>({
      resolver: zodResolver(changePasswordSchema),
      defaultValues: {
        currentPassword: '',
        newPassword: '',
        confirmPassword: '',
      },
    })

  const onSubmit = handleSubmit((values) => {
    changePassword.mutate(values, {
      // Clears the form back to empty on a real success, an event-handler
      // callback rather than a render-time side effect, the same
      // "setState in response to the mutation settling, not during
      // render" shape LoginForm's countdown effect already follows.
      onSuccess: () => {
        reset()
      },
    })
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>Change password</CardTitle>
        <CardDescription>
          Changing your password signs out every other active session for this
          account. This browser stays signed in.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <FieldGroup>
            <Field
              data-invalid={formState.errors.currentPassword ? true : undefined}
            >
              <FieldLabel htmlFor="account-current-password">
                Current password
              </FieldLabel>
              <Input
                id="account-current-password"
                type="password"
                autoComplete="current-password"
                aria-invalid={!!formState.errors.currentPassword}
                {...register('currentPassword')}
              />
              <FieldError errors={[formState.errors.currentPassword]} />
            </Field>
            <Field
              data-invalid={formState.errors.newPassword ? true : undefined}
            >
              <FieldLabel htmlFor="account-new-password">
                New password
              </FieldLabel>
              <Input
                id="account-new-password"
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
              <FieldLabel htmlFor="account-confirm-password">
                Confirm new password
              </FieldLabel>
              <Input
                id="account-confirm-password"
                type="password"
                autoComplete="new-password"
                aria-invalid={!!formState.errors.confirmPassword}
                {...register('confirmPassword')}
              />
              <FieldError errors={[formState.errors.confirmPassword]} />
            </Field>
          </FieldGroup>

          {changePassword.isSuccess ? (
            <Alert>
              <CheckCircleIcon />
              <AlertTitle>Password changed</AlertTitle>
              <AlertDescription>
                Your other sessions have been signed out. This one stays signed
                in.
              </AlertDescription>
            </Alert>
          ) : changePassword.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>
                {changePassword.error.message}
              </AlertDescription>
            </Alert>
          ) : null}

          <Button type="submit" disabled={changePassword.isPending}>
            {changePassword.isPending
              ? 'Changing password...'
              : 'Change password'}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
