import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  EyeIcon,
  EyeSlashIcon,
  LockIcon,
  LockOpenIcon,
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import {
  SecretsNotConfiguredError,
  useSecretKeys,
  useSetSecret,
  useSetSecretLock,
} from '../queries/secrets'

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
  const secretKeysQuery = useSecretKeys(appName)
  const setSecretLock = useSetSecretLock(appName)
  const [pendingOverwrite, setPendingOverwrite] = useState<{
    key: string
    value: string
  } | null>(null)
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
          toast.add({ title: 'Secret saved.', type: 'success' })
        },
        onError: (error) => {
          if (error instanceof SecretsNotConfiguredError) {
            setNotConfigured(true)
            return
          }
          if (error instanceof ApiError && error.status === 409) {
            setPendingOverwrite({
              key: values.key.trim(),
              value: values.value,
            })
          }
        },
      },
    )
  })

  const confirmOverwrite = () => {
    if (!pendingOverwrite) return
    setSecret.mutate(
      { ...pendingOverwrite, overwriteLocked: true },
      {
        onSuccess: () => {
          reset({ key: '', value: '' })
          setRevealValue(false)
          setPendingOverwrite(null)
          toast.add({ title: 'Secret saved.', type: 'success' })
        },
      },
    )
  }

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
        {secretKeysQuery.data && secretKeysQuery.data.length > 0 ? (
          <ul className="mb-4 space-y-1 rounded-md border border-border p-2">
            {secretKeysQuery.data.map((k) => (
              <li
                key={k.key}
                className="flex items-center justify-between gap-2 rounded px-2 py-1 text-sm"
              >
                <span className="font-mono">{k.key}</span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={setSecretLock.isPending}
                  onClick={() => {
                    setSecretLock.mutate({ key: k.key, locked: !k.locked })
                  }}
                  aria-label={k.locked ? `Unlock ${k.key}` : `Lock ${k.key}`}
                  aria-pressed={k.locked}
                >
                  {k.locked ? (
                    <LockIcon className="size-4" />
                  ) : (
                    <LockOpenIcon className="size-4 text-muted-foreground" />
                  )}
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
        </form>
      </CardContent>
      <Dialog
        open={pendingOverwrite !== null}
        onOpenChange={(next) => {
          if (!next) setPendingOverwrite(null)
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{pendingOverwrite?.key} is locked</DialogTitle>
            <DialogDescription>
              Overwrite its value anyway? This cannot be undone: the previous
              value is not recoverable once replaced.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setPendingOverwrite(null)
              }}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={setSecret.isPending}
              onClick={confirmOverwrite}
            >
              {setSecret.isPending ? 'Saving...' : 'Overwrite'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
