import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { TrashIcon, LockIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useClearAppVaultEnv,
  useSetAppVaultEnv,
} from '../queries/appVaultEnv'

const vaultEnvSchema = z.object({
  key: z.string().trim().min(1, 'Key is required'),
  path: z.string().trim().min(1, 'Vault path is required'),
  vaultKey: z.string().trim().min(1, 'Field is required'),
})

type VaultEnvFormValues = z.infer<typeof vaultEnvSchema>

// Declares (or removes) one env var as resolving live from an external
// HashiCorp Vault instance, the UI counterpart to app.yaml's own
// { vault: { path, key } } env var syntax for an app that already
// exists. Deliberately not built on SecretsEditor: there is no value to
// type or mask here, only a { path, key } reference, and it writes
// through a different endpoint (PUT/DELETE .../vault-env/{key}, never
// .../secrets/{key}) that never touches internal/secrets at all.
export function VaultEnvEditor({
  appName,
  vaultEnv,
}: {
  appName: string
  vaultEnv: Record<string, { path: string; key: string }> | undefined
}) {
  const setVaultEnv = useSetAppVaultEnv(appName)
  const clearVaultEnv = useClearAppVaultEnv(appName)
  const { register, handleSubmit, formState, reset } =
    useForm<VaultEnvFormValues>({
      resolver: zodResolver(vaultEnvSchema),
      defaultValues: { key: '', path: '', vaultKey: '' },
    })

  const onSubmit = handleSubmit((values) => {
    setVaultEnv.mutate(
      { key: values.key.trim(), path: values.path.trim(), vaultKey: values.vaultKey.trim() },
      {
        onSuccess: () => {
          reset({ key: '', path: '', vaultKey: '' })
          toast.add({ title: 'Vault-sourced env var saved.', type: 'success' })
        },
      },
    )
  })

  const entries = Object.entries(vaultEnv ?? {})

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockIcon className="size-4" />
          Vault-sourced env vars
        </CardTitle>
        <CardDescription>
          Resolves an env var&apos;s value live from an external HashiCorp
          Vault instance at deploy time instead of a value this platform
          stores. Requires Vault to be configured under Settings first.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {entries.length > 0 ? (
          <ul className="mb-4 space-y-1 rounded-md border border-border p-2">
            {entries.map(([key, ref]) => (
              <li
                key={key}
                className="flex items-center justify-between gap-2 rounded px-2 py-1 text-sm"
              >
                <span className="font-mono">
                  {key}
                  <span className="ml-2 text-muted-foreground">
                    {ref.path}#{ref.key}
                  </span>
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={clearVaultEnv.isPending}
                  onClick={() => {
                    clearVaultEnv.mutate(key)
                  }}
                  aria-label={`Remove ${key}`}
                >
                  <TrashIcon className="size-4 text-muted-foreground" />
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
              <FieldLabel htmlFor="vault-env-key">Env var</FieldLabel>
              <Input
                id="vault-env-key"
                className="font-mono"
                placeholder="API_KEY"
                autoComplete="off"
                autoCapitalize="off"
                spellCheck={false}
                {...register('key')}
              />
              <FieldError
                errors={formState.errors.key ? [formState.errors.key] : undefined}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="vault-env-path">Vault path</FieldLabel>
              <Input
                id="vault-env-path"
                className="font-mono"
                placeholder="myapp/config"
                autoComplete="off"
                {...register('path')}
              />
              <FieldError
                errors={formState.errors.path ? [formState.errors.path] : undefined}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="vault-env-field">Field</FieldLabel>
              <Input
                id="vault-env-field"
                className="font-mono"
                placeholder="api_key"
                autoComplete="off"
                {...register('vaultKey')}
              />
              <FieldError
                errors={
                  formState.errors.vaultKey ? [formState.errors.vaultKey] : undefined
                }
              />
            </Field>
          </FieldGroup>
          <div className="mt-3">
            <Button type="submit" size="sm" disabled={setVaultEnv.isPending}>
              {setVaultEnv.isPending ? 'Saving...' : 'Save'}
            </Button>
          </div>
          {setVaultEnv.isError ? (
            <Alert variant="destructive" className="mt-3">
              <AlertDescription>{setVaultEnv.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
