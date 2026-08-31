import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  EyeIcon,
  EyeSlashIcon,
  PackageIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import { useBrand } from '../hooks/useBrand'
import { useCreateRegistryCredential } from '../queries/registryCredentials'

// Mirrors validateCreateRegistryCredentialRequest (internal/api/
// registry_credentials.go): name/registry_host/username/password are
// all always required. Client-side fast feedback only, same reasoning
// createBackupTargetSchema's own comment gives elsewhere.
const createRegistryCredentialSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  registry_host: z.string().trim().min(1, 'Registry host is required'),
  username: z.string().trim().min(1, 'Username is required'),
  password: z.string().min(1, 'Password is required'),
  // A plain <input type="date"> value ("" or "YYYY-MM-DD"), converted to
  // a full RFC3339 timestamp on submit. Optional: this platform cannot
  // read an expiry out of an opaque credential string, so it's only ever
  // what the operator already knows and chooses to record.
  expires_at: z.string(),
})

type CreateRegistryCredentialFormValues = z.infer<
  typeof createRegistryCredentialSchema
>

const defaultValues: CreateRegistryCredentialFormValues = {
  name: '',
  registry_host: '',
  username: '',
  password: '',
  expires_at: '',
}

// Adds a username/password pair for pulling a private image, referenced
// by name from app.yaml's build.registryCredential field. POST
// /api/v1/registry-credentials requires write:sensitive, the same
// ability tier CreateBackupTargetDialog's own doc comment explains for
// the identical reason (a live credential in the request body).
export function CreateRegistryCredentialDialog() {
  const brand = useBrand()
  const displayName = brand.ShortName || brand.Name
  const [open, setOpen] = useState(false)
  const [revealPassword, setRevealPassword] = useState(false)
  const createCredential = useCreateRegistryCredential()
  const { register, handleSubmit, formState, reset } =
    useForm<CreateRegistryCredentialFormValues>({
      resolver: zodResolver(createRegistryCredentialSchema),
      defaultValues,
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      setRevealPassword(false)
      createCredential.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    createCredential.mutate(
      {
        name: values.name.trim(),
        registry_host: values.registry_host.trim(),
        username: values.username.trim(),
        password: values.password,
        expires_at: values.expires_at
          ? new Date(values.expires_at).toISOString()
          : undefined,
      },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: `Registry credential "${created.name}" added.`,
            type: 'success',
          })
        },
      },
    )
  })

  const notConfigured =
    createCredential.isError &&
    createCredential.error instanceof ApiError &&
    createCredential.error.status === 501

  const generalError =
    createCredential.isError && !notConfigured
      ? createCredential.error.message
      : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Add credential</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {notConfigured ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <PackageIcon className="size-4 text-muted-foreground" />
                Add credential
              </DialogTitle>
            </DialogHeader>
            <Alert variant="destructive">
              <AlertTitle>
                Registry credentials are not configured on this server
              </AlertTitle>
              <AlertDescription>
                The control plane was started without APP_MASTER_KEY set, so
                it cannot encrypt or store registry passwords. Set
                APP_MASTER_KEY and restart the control plane to enable this.
              </AlertDescription>
            </Alert>
            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Close
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <PackageIcon className="size-4 text-muted-foreground" />
                Add credential
              </DialogTitle>
              <DialogDescription>
                Referenced by name from app.yaml's build.registryCredential
                field, for pulling a private image with build.type: image.
              </DialogDescription>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                void onSubmit(e)
              }}
              className="space-y-4"
            >
              <Field>
                <FieldLabel htmlFor="registry-credential-name">
                  Name
                </FieldLabel>
                <Input
                  id="registry-credential-name"
                  placeholder="e.g. ghcr-bot"
                  {...register('name')}
                />
                <FieldError errors={[formState.errors.name]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="registry-credential-host">
                  Registry host
                </FieldLabel>
                <Input
                  id="registry-credential-host"
                  className="font-mono"
                  placeholder="ghcr.io"
                  {...register('registry_host')}
                />
                <FieldError errors={[formState.errors.registry_host]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="registry-credential-username">
                  Username
                </FieldLabel>
                <Input
                  id="registry-credential-username"
                  autoComplete="off"
                  autoCapitalize="off"
                  spellCheck={false}
                  {...register('username')}
                />
                <FieldError errors={[formState.errors.username]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="registry-credential-password">
                  Password{' '}
                  <span className="text-muted-foreground">
                    (or access token)
                  </span>
                </FieldLabel>
                <div className="relative">
                  <Input
                    id="registry-credential-password"
                    type={revealPassword ? 'text' : 'password'}
                    className="pr-9 font-mono"
                    autoComplete="off"
                    {...register('password')}
                  />
                  <button
                    type="button"
                    onClick={() => {
                      setRevealPassword((v) => !v)
                    }}
                    aria-label={revealPassword ? 'Hide value' : 'Show value'}
                    aria-pressed={revealPassword}
                    className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground hover:text-foreground"
                  >
                    {revealPassword ? (
                      <EyeSlashIcon className="size-4" />
                    ) : (
                      <EyeIcon className="size-4" />
                    )}
                  </button>
                </div>
                <FieldError errors={[formState.errors.password]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="registry-credential-expires-at">
                  Expires{' '}
                  <span className="text-muted-foreground">(optional)</span>
                </FieldLabel>
                <Input
                  id="registry-credential-expires-at"
                  type="date"
                  {...register('expires_at')}
                />
                <p className="text-xs text-muted-foreground">
                  Only if you already know the expiry, e.g. a GitHub PAT or a
                  cloud registry's short-lived token. {displayName} can't
                  detect this on its own.
                </p>
                <FieldError errors={[formState.errors.expires_at]} />
              </Field>

              {generalError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>{generalError}</AlertDescription>
                </Alert>
              ) : null}

              <DialogFooter>
                <Button type="submit" disabled={createCredential.isPending}>
                  {createCredential.isPending ? 'Adding...' : 'Add credential'}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
