import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { CheckCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useChangePassword } from '../queries/account'
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
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from './ui/card'

// Split out of routes/settings/account.tsx (see that file's history): a
// route file that imports 'zod' at module scope defeats TanStack Router's
// autoCodeSplitting for that import specifically, because the splitter
// only cleanly separates a route file's own component/loader/pending/
// error exports into their own chunk, not arbitrary same-file helper
// consts (changePasswordSchema) a component closes over. With the schema
// and zod import living in this file instead, `settings/account.tsx`
// only ever reaches zod through this component, which the route's
// generated split module already isolates, so zod stops getting
// duplicated into the always-loaded main chunk (it was appearing there
// verbatim, in addition to the shared `zod` chunk every dynamically
// split form already shares, per the bundle-analysis pass that added
// rollup-plugin-visualizer).
//
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
export function ChangePasswordCard() {
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
