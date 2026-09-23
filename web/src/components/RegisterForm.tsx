import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useRegister } from '../queries/auth'
import { useBrand } from '../hooks/useBrand'
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

const registerSchema = z
  .object({
    setupToken: z.string().trim().min(1, 'Setup token is required'),
    username: z.string().trim().min(1, 'Username is required'),
    // Mirrors internal/api/auth.go's minPasswordLength (8) exactly: this
    // is a client-side head start, not a substitute for the server's own
    // check, which still runs and still wins if the two ever drift.
    password: z.string().min(8, 'Password must be at least 8 characters'),
    confirmPassword: z.string(),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: 'Passwords do not match',
    path: ['confirmPassword'],
  })

type RegisterFormValues = z.infer<typeof registerSchema>

// First-run admin setup. The setup token comes from the installer's
// summary or the control plane's setup-token subcommand; a 409 means an
// admin already exists, so this offers to switch to Sign in.
export function RegisterForm({
  onSwitchToSignIn,
  initialSetupToken = '',
}: {
  onSwitchToSignIn: () => void
  initialSetupToken?: string
}) {
  const registerAdmin = useRegister()
  const brand = useBrand()
  const { register, handleSubmit, formState } = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: {
      setupToken: initialSetupToken,
      username: '',
      password: '',
      confirmPassword: '',
    },
  })

  const onSubmit = handleSubmit((values) => {
    registerAdmin.mutate(values)
  })

  const adminAlreadyExists =
    registerAdmin.isError && registerAdmin.error.status === 409

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      className="mt-4 space-y-4"
    >
      <FieldGroup>
        <Field data-invalid={formState.errors.setupToken ? true : undefined}>
          <FieldLabel htmlFor="register-setup-token">Setup token</FieldLabel>
          <Input
            id="register-setup-token"
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
            aria-invalid={!!formState.errors.setupToken}
            {...register('setupToken')}
          />
          {formState.errors.setupToken ? (
            <FieldError errors={[formState.errors.setupToken]} />
          ) : (
            <FieldDescription>
              Printed at the end of the install. Run{' '}
              <code className="font-mono">
                sudo {brand.BinaryName} setup-token
              </code>{' '}
              on the server to see it again.
            </FieldDescription>
          )}
        </Field>
        <Field data-invalid={formState.errors.username ? true : undefined}>
          <FieldLabel htmlFor="register-username">Username</FieldLabel>
          <Input
            id="register-username"
            autoComplete="username"
            aria-invalid={!!formState.errors.username}
            {...register('username')}
          />
          <FieldError errors={[formState.errors.username]} />
        </Field>
        <Field data-invalid={formState.errors.password ? true : undefined}>
          <FieldLabel htmlFor="register-password">Password</FieldLabel>
          <Input
            id="register-password"
            type="password"
            autoComplete="new-password"
            aria-invalid={!!formState.errors.password}
            {...register('password')}
          />
          {formState.errors.password ? (
            <FieldError errors={[formState.errors.password]} />
          ) : (
            <FieldDescription>At least 8 characters.</FieldDescription>
          )}
        </Field>
        <Field
          data-invalid={formState.errors.confirmPassword ? true : undefined}
        >
          <FieldLabel htmlFor="register-confirm-password">
            Confirm password
          </FieldLabel>
          <Input
            id="register-confirm-password"
            type="password"
            autoComplete="new-password"
            aria-invalid={!!formState.errors.confirmPassword}
            {...register('confirmPassword')}
          />
          <FieldError errors={[formState.errors.confirmPassword]} />
        </Field>
      </FieldGroup>

      {adminAlreadyExists ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertTitle>An admin account already exists</AlertTitle>
          <AlertDescription>
            <button
              type="button"
              onClick={onSwitchToSignIn}
              className="underline underline-offset-4"
            >
              Switch to Sign in
            </button>
          </AlertDescription>
        </Alert>
      ) : registerAdmin.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{registerAdmin.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="submit"
        className="w-full"
        disabled={registerAdmin.isPending}
      >
        {registerAdmin.isPending
          ? 'Creating account...'
          : 'Create admin account'}
      </Button>
    </form>
  )
}
