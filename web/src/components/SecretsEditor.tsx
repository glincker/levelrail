import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { EyeIcon, EyeSlashIcon, LockIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { SecretsNotConfiguredError, useSetSecret } from '../queries/secrets'

const secretSchema = z.object({
  key: z.string().trim().min(1, 'Key is required'),
  value: z.string().min(1, 'Value is required'),
})

type SecretFormValues = z.infer<typeof secretSchema>

// Sets (or rotates) one env var's encrypted value via PUT
// /api/v1/apps/{name}/secrets/{key} (internal/api/secrets.go,
// TASKS.md 1.7). Deliberately not built on EnvEditor or its
// AppDetail.env map: this is a separate write path to a separate
// backend endpoint with no corresponding GET, so there is nothing to
// list or pre-populate here, only a "set a value" form that clears
// itself on success. See docs-local/research/dashboard-gap-audit-and-
// devmode.md gap #5 for the full gap this closes.
//
// The value field defaults to a masked (type="password") input with a
// reveal toggle rather than plain text, since a secret is being typed
// here even though it is never displayed once saved.
export function SecretsEditor({ appName }: { appName: string }) {
  const [revealValue, setRevealValue] = useState(false)
  const [notConfigured, setNotConfigured] = useState(false)
  const setSecret = useSetSecret(appName)
  const { register, handleSubmit, formState, reset } =
    useForm<SecretFormValues>({
      resolver: zodResolver(secretSchema),
      defaultValues: { key: '', value: '' },
    })

  const onSubmit = handleSubmit((values) => {
    setSecret.mutate(
      { key: values.key.trim(), value: values.value },
      {
        onSuccess: () => {
          reset({ key: '', value: '' })
          setRevealValue(false)
        },
        onError: (error) => {
          if (error instanceof SecretsNotConfiguredError) {
            setNotConfigured(true)
          }
        },
      },
    )
  })

  // Once a 501 has been seen, the server-side gap is not something a
  // different key/value or a retry fixes: hide the form entirely
  // rather than let an operator resubmit into the same wall. There is
  // no GET this component could poll to learn the master key was
  // configured after the fact; a page reload re-checks on next submit.
  if (notConfigured) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <LockIcon className="size-4" />
            Secrets
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

  const generalError =
    setSecret.isError && !(setSecret.error instanceof SecretsNotConfiguredError)
      ? setSecret.error.message
      : null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockIcon className="size-4" />
          Secrets
        </CardTitle>
        <CardDescription>
          Sets an encrypted value for one env var by key, e.g. API_KEY. There is
          no way to view a secret once saved: this only writes a value, it never
          reads one back.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="secret-key">Key</FieldLabel>
              <Input
                id="secret-key"
                className="font-mono"
                placeholder="API_KEY"
                autoComplete="off"
                autoCapitalize="off"
                spellCheck={false}
                {...register('key')}
              />
              <FieldError
                errors={
                  formState.errors.key ? [formState.errors.key] : undefined
                }
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="secret-value">Value</FieldLabel>
              <div className="relative">
                <Input
                  id="secret-value"
                  type={revealValue ? 'text' : 'password'}
                  className="pr-9 font-mono"
                  placeholder="secret value"
                  autoComplete="off"
                  {...register('value')}
                />
                <button
                  type="button"
                  onClick={() => {
                    setRevealValue((v) => !v)
                  }}
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
              <FieldError
                errors={
                  formState.errors.value ? [formState.errors.value] : undefined
                }
              />
            </Field>
          </FieldGroup>
          <div className="mt-3">
            <Button type="submit" size="sm" disabled={setSecret.isPending}>
              {setSecret.isPending ? 'Saving...' : 'Save secret'}
            </Button>
          </div>
          {generalError ? (
            <Alert variant="destructive" className="mt-3">
              <AlertDescription>{generalError}</AlertDescription>
            </Alert>
          ) : null}
          {setSecret.isSuccess ? (
            <p className="mt-2 text-xs text-green-700 dark:text-green-400">
              Secret saved.
            </p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
