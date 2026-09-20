import type { FieldErrors, UseFormRegister } from 'react-hook-form'
import { EyeIcon, EyeSlashIcon, LockIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { SecretKeyValueFormValues } from '@/lib/secretKeyValueSchema'

// Shown by SecretsEditor and SharedEnvSecretsCard in place of their own
// form once a 501 has been seen: the server-side gap (no APP_MASTER_KEY)
// isn't something a different key/value or a retry fixes, and there is no
// GET either could poll to learn the master key was configured after the
// fact, so a page reload is what re-checks on next submit.
export function SecretNotConfiguredCard({ title }: { title: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockIcon className="size-4" />
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Alert variant="destructive">
          <AlertTitle>Secrets are not configured on this server</AlertTitle>
          <AlertDescription>
            The control plane was started without APP_MASTER_KEY set, so it
            cannot encrypt or store secret values. Set APP_MASTER_KEY and
            restart the control plane to enable this.
          </AlertDescription>
        </Alert>
      </CardContent>
    </Card>
  )
}

// Key + masked-value-with-reveal-toggle fields, shared by SecretsEditor
// (an app's own secrets) and SharedEnvSecretsCard (project/organization/
// environment-scoped secret shared vars): identical input shape, only the
// surrounding card, key list, and submit mutation differ per caller.
// revealValue/onToggleReveal are controlled by the caller so it can reset
// the toggle to hidden alongside its own form reset on a successful save.
export function SecretKeyValueFields({
  idPrefix,
  register,
  errors,
  revealValue,
  onToggleReveal,
}: {
  idPrefix: string
  register: UseFormRegister<SecretKeyValueFormValues>
  errors: FieldErrors<SecretKeyValueFormValues>
  revealValue: boolean
  onToggleReveal: () => void
}) {
  return (
    <FieldGroup>
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-key`}>Key</FieldLabel>
        <Input
          id={`${idPrefix}-key`}
          className="font-mono"
          placeholder="API_KEY"
          autoComplete="off"
          autoCapitalize="off"
          spellCheck={false}
          {...register('key')}
        />
        <FieldError errors={errors.key ? [errors.key] : undefined} />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-value`}>Value</FieldLabel>
        <div className="relative">
          <Input
            id={`${idPrefix}-value`}
            type={revealValue ? 'text' : 'password'}
            className="pr-9 font-mono"
            placeholder="secret value"
            autoComplete="off"
            {...register('value')}
          />
          <button
            type="button"
            onClick={onToggleReveal}
            aria-label={revealValue ? 'Hide value' : 'Show value'}
            aria-pressed={revealValue}
            className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground hover:text-foreground"
          >
            {revealValue ? (
              <EyeSlashIcon className="size-4" />
            ) : (
              <EyeIcon className="size-4" />
            )}
          </button>
        </div>
        <FieldError errors={errors.value ? [errors.value] : undefined} />
      </Field>
    </FieldGroup>
  )
}
