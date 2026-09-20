import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  EyeIcon,
  EyeSlashIcon,
  LockIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import { SecretsNotConfiguredError } from '../queries/secrets'
import {
  useDeleteSharedEnvSecret,
  useSetSharedEnvSecret,
  useSharedEnvAll,
  type SharedEnvScope,
} from '../queries/sharedEnv'

const secretSchema = z.object({
  key: z.string().trim().min(1, 'Key is required'),
  value: z.string().min(1, 'Value is required'),
})

type SecretFormValues = z.infer<typeof secretSchema>

const scopeLabel: Record<SharedEnvScope, string> = {
  project: 'project',
  organization: 'organization',
  environment: 'environment',
}

// Manages secret-marked shared env vars at a project/organization/
// environment scope: encrypted the same way an app's own SecretsEditor
// handles { secret: true } env vars (internal/secrets, reused unchanged
// via PUT/DELETE .../env/secrets/{key}), every app filed under this
// scope inherits the decrypted value automatically at deploy time with
// no action needed on the app's own side (internal/reconcile/
// application's resolveEnv, via internal/sharedenv.Resolver). A plain
// (non-secret) shared var has its own separate full-replace editor per
// scope (OrganizationEnvEditor, EnvironmentEnvEditor, and the project
// equivalent); this card is deliberately scoped to the secret case only,
// so a full-replace PUT to the plain .../env endpoint can never
// accidentally wipe a secret-marked row (see store.DB.
// Set{Project,Organization,Environment}EnvVars' own is_secret-scoped
// DELETE clause).
export function SharedEnvSecretsCard({
  scope,
  id,
}: {
  scope: SharedEnvScope
  id: string
}) {
  const [revealValue, setRevealValue] = useState(false)
  const [notConfigured, setNotConfigured] = useState(false)
  const allQuery = useSharedEnvAll(scope, id)
  const setSecret = useSetSharedEnvSecret(scope, id)
  const deleteSecret = useDeleteSharedEnvSecret(scope, id)
  const { register, handleSubmit, formState, reset } =
    useForm<SecretFormValues>({
      resolver: zodResolver(secretSchema),
      defaultValues: { key: '', value: '' },
    })

  const secretKeys = (allQuery.data ?? []).filter((v) => v.secret)

  const onSubmit = handleSubmit((values) => {
    setSecret.mutate(
      { key: values.key.trim(), value: values.value },
      {
        onSuccess: () => {
          reset({ key: '', value: '' })
          setRevealValue(false)
          toast.add({ title: 'Secret shared variable saved.', type: 'success' })
        },
        onError: (error) => {
          if (error instanceof SecretsNotConfiguredError) {
            setNotConfigured(true)
          }
        },
      },
    )
  })

  if (notConfigured) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <LockIcon className="size-4" />
            Secret shared variables
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
      : deleteSecret.isError
        ? (deleteSecret.error as ApiError).message
        : null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockIcon className="size-4" />
          Secret shared variables
        </CardTitle>
        <CardDescription>
          Encrypted values shared with every app filed under this{' '}
          {scopeLabel[scope]}, resolved fresh at deploy time. There is no way to
          view a value once saved: this only writes a value, it never reads one
          back.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {secretKeys.length > 0 ? (
          <ul className="mb-4 space-y-1 rounded-md border border-border p-2">
            {secretKeys.map((k) => (
              <li
                key={k.key}
                className="flex items-center justify-between gap-2 rounded px-2 py-1 text-sm"
              >
                <span className="font-mono">{k.key}</span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={deleteSecret.isPending}
                  onClick={() => {
                    deleteSecret.mutate(k.key)
                  }}
                  aria-label={`Remove ${k.key}`}
                >
                  <XIcon className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor={`${scope}-${id}-shared-secret-key`}>
                Key
              </FieldLabel>
              <Input
                id={`${scope}-${id}-shared-secret-key`}
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
              <FieldLabel htmlFor={`${scope}-${id}-shared-secret-value`}>
                Value
              </FieldLabel>
              <div className="relative">
                <Input
                  id={`${scope}-${id}-shared-secret-value`}
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
              {setSecret.isPending ? 'Saving...' : 'Save secret variable'}
            </Button>
          </div>
          {generalError ? (
            <Alert variant="destructive" className="mt-3">
              <AlertDescription>{generalError}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
