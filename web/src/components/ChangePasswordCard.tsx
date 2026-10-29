import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { z } from 'zod'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { useChangePassword } from '../queries/account'
import { useSession } from '../queries/security'
import { MIN_PASSWORD_LENGTH } from '../lib/passwordStrength'
import { PasswordStrengthMeter } from './account/PasswordStrengthMeter'
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

// Own file so autoCodeSplitting keeps zod out of the always-loaded bundle.
// MIN_PASSWORD_LENGTH mirrors internal/api/account.go's minPasswordLength;
// the server check still wins if the two drift.
function buildChangePasswordSchema(
  requireCurrentPassword: boolean,
  t: TFunction<'settings'>,
) {
  return z
    .object({
      currentPassword: requireCurrentPassword
        ? z.string().min(1, t('accountPage.password.currentRequired'))
        : z.string(),
      newPassword: z
        .string()
        .min(
          MIN_PASSWORD_LENGTH,
          t('accountPage.password.tooShort', { min: MIN_PASSWORD_LENGTH }),
        ),
      confirmPassword: z.string(),
    })
    .refine((data) => data.newPassword === data.confirmPassword, {
      message: t('accountPage.password.mismatch'),
      path: ['confirmPassword'],
    })
}

type ChangePasswordValues = z.infer<
  ReturnType<typeof buildChangePasswordSchema>
>

// The server collapses "no session" and "wrong current password" into one
// 401 on purpose, so the form surfaces its message as-is.
export function ChangePasswordCard() {
  const { t } = useTranslation('settings')
  const changePassword = useChangePassword()
  const { data: session } = useSession()
  const schema = useMemo(
    () => buildChangePasswordSchema(session.has_password, t),
    [session.has_password, t],
  )
  const { register, handleSubmit, formState, reset, watch } =
    useForm<ChangePasswordValues>({
      resolver: zodResolver(schema),
      defaultValues: {
        currentPassword: '',
        newPassword: '',
        confirmPassword: '',
      },
    })
  const newPassword = watch('newPassword')

  const onSubmit = handleSubmit((values) => {
    changePassword.mutate(values, {
      onSuccess: () => {
        reset()
      },
    })
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {session.has_password
            ? t('accountPage.password.titleChange')
            : t('accountPage.password.titleSet')}
        </CardTitle>
        <CardDescription>
          {session.has_password
            ? t('accountPage.password.descChange')
            : t('accountPage.password.descSet')}
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
            {session.has_password ? (
              <Field
                data-invalid={
                  formState.errors.currentPassword ? true : undefined
                }
              >
                <FieldLabel htmlFor="account-current-password">
                  {t('accountPage.password.current')}
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
            ) : null}
            <Field
              data-invalid={formState.errors.newPassword ? true : undefined}
            >
              <FieldLabel htmlFor="account-new-password">
                {t('accountPage.password.new')}
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
                  {t('accountPage.password.minHint', {
                    min: MIN_PASSWORD_LENGTH,
                  })}
                </FieldDescription>
              )}
              <PasswordStrengthMeter password={newPassword} />
            </Field>
            <Field
              data-invalid={formState.errors.confirmPassword ? true : undefined}
            >
              <FieldLabel htmlFor="account-confirm-password">
                {t('accountPage.password.confirm')}
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
              <CircleCheckGlassIcon
                className="text-tone-success"
                size="1.4em"
              />
              <AlertTitle>{t('accountPage.password.changed')}</AlertTitle>
              <AlertDescription>
                {t('accountPage.password.changedBody')}
              </AlertDescription>
            </Alert>
          ) : changePassword.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>
                {changePassword.error.message}
              </AlertDescription>
            </Alert>
          ) : null}

          <Button type="submit" disabled={changePassword.isPending}>
            {changePassword.isPending
              ? t('accountPage.password.saving')
              : session.has_password
                ? t('accountPage.password.submitChange')
                : t('accountPage.password.submitSet')}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
