import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { LockIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import { SecretsNotConfiguredError } from '../queries/secrets'
import {
  useDeleteSharedEnvSecret,
  useSetSharedEnvSecret,
  useSharedEnvAll,
  type SharedEnvScope,
} from '../queries/sharedEnv'
import {
  SecretAgeBadge,
  SecretKeyValueFields,
  SecretNotConfiguredCard,
} from './SecretKeyValueFields'
import {
  secretKeyValueSchema,
  type SecretKeyValueFormValues,
} from '@/lib/secretKeyValueSchema'

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
    useForm<SecretKeyValueFormValues>({
      resolver: zodResolver(secretKeyValueSchema),
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
    return <SecretNotConfiguredCard title="Secret shared variables" />
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
                <span className="flex min-w-0 flex-col gap-0.5">
                  <span className="font-mono">{k.key}</span>
                  <SecretAgeBadge updatedAt={k.updatedAt} stale={k.stale} />
                </span>
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
          <SecretKeyValueFields
            idPrefix={`${scope}-${id}-shared-secret`}
            register={register}
            errors={formState.errors}
            revealValue={revealValue}
            onToggleReveal={() => {
              setRevealValue((v) => !v)
            }}
          />
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
