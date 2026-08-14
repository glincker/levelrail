import { useEffect, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  CloudArrowUpIcon,
  EyeIcon,
  EyeSlashIcon,
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import { useCreateBackupTarget } from '../queries/backupTargets'
import { PROVIDER_LABEL } from './backupTargetProvider'
import type { BackupProvider } from '../types/backupTarget'

// Mirrors validateCreateBackupTargetRequest (internal/api/
// backup_targets.go) so an obviously-invalid submission never round-
// trips to the server just to bounce back a 400: name, bucket, and both
// credential fields are always required; endpoint is required only for
// r2/custom (aws never needs one, since AWS S3 has a well-known
// endpoint scheme the control plane fills in itself). This is
// client-side fast feedback only, same reasoning createDatabaseSchema's
// own comment gives elsewhere: the real request still goes out on
// submit and the real response is what onSubmit below shows.
const createBackupTargetSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    provider: z.enum(['aws', 'r2', 'custom']),
    bucket: z.string().trim().min(1, 'Bucket is required'),
    region: z.string().trim().optional(),
    endpoint: z.string().trim().optional(),
    access_key_id: z.string().trim().min(1, 'Access key ID is required'),
    secret_access_key: z.string().min(1, 'Secret access key is required'),
  })
  .refine(
    (values) => values.provider === 'aws' || (values.endpoint ?? '').length > 0,
    {
      message: 'Endpoint is required for R2 and custom providers',
      path: ['endpoint'],
    },
  )

type CreateBackupTargetFormValues = z.infer<typeof createBackupTargetSchema>

const defaultValues: CreateBackupTargetFormValues = {
  name: '',
  provider: 'aws',
  bucket: '',
  region: '',
  endpoint: '',
  access_key_id: '',
  secret_access_key: '',
}

const ENDPOINT_PLACEHOLDER: Record<BackupProvider, string> = {
  aws: '',
  r2: 'https://<account-id>.r2.cloudflarestorage.com',
  custom: 'https://s3.example.com',
}

// Connects a new S3-compatible bucket as a backup destination. POST
// /api/v1/backup-targets (handleCreateBackupTarget) requires
// write:sensitive, same ability PUT /apps/{name}/secrets/{key} needs:
// nothing here hides or disables the trigger based on the current
// session's abilities though, matching how the rest of the dashboard
// handles ability-gated actions (queries/nodes.ts's own comment: the
// dashboard's session is never scoped down, a 403 is a real, surfaced
// error, not something the UI pre-empts by hiding buttons).
export function CreateBackupTargetDialog() {
  const [open, setOpen] = useState(false)
  const [revealSecret, setRevealSecret] = useState(false)
  const createBackupTarget = useCreateBackupTarget()
  const { control, register, handleSubmit, formState, reset, setValue, watch } =
    useForm<CreateBackupTargetFormValues>({
      resolver: zodResolver(createBackupTargetSchema),
      defaultValues,
    })
  const watchedProvider = watch('provider')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      setRevealSecret(false)
      createBackupTarget.reset()
    }
  }

  // Endpoint is only ever meaningful for r2/custom (aws must omit it
  // entirely per validateCreateBackupTargetRequest's own check), so
  // switching back to aws clears whatever was typed rather than leaving
  // a stale value that onSubmit would just have to strip anyway.
  useEffect(() => {
    if (watchedProvider === 'aws') {
      setValue('endpoint', '')
    }
    // Only reacting to the provider field's own value, not to setValue's
    // identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [watchedProvider])

  const onSubmit = handleSubmit((values) => {
    createBackupTarget.mutate(
      {
        name: values.name.trim(),
        provider: values.provider,
        bucket: values.bucket.trim(),
        ...(values.region?.trim() ? { region: values.region.trim() } : {}),
        ...(values.provider !== 'aws' && values.endpoint?.trim()
          ? { endpoint: values.endpoint.trim() }
          : {}),
        access_key_id: values.access_key_id.trim(),
        secret_access_key: values.secret_access_key,
      },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: `Backup target "${created.name}" connected.`,
            type: 'success',
          })
        },
      },
    )
  })

  const notConfigured =
    createBackupTarget.isError &&
    createBackupTarget.error instanceof ApiError &&
    createBackupTarget.error.status === 501

  const generalError =
    createBackupTarget.isError && !notConfigured
      ? createBackupTarget.error.message
      : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Connect bucket</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {notConfigured ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <CloudArrowUpIcon className="size-4 text-muted-foreground" />
                Connect bucket
              </DialogTitle>
            </DialogHeader>
            {/* Same "server configuration gap, not a bad submission"
                treatment SecretsEditor gives an unconfigured master key:
                there is nothing a different name/bucket/credential set
                would fix here, so this replaces the form entirely rather
                than leaving it open to a retry that would just 501
                again. */}
            <Alert variant="destructive">
              <AlertTitle>
                Backup targets are not configured on this server
              </AlertTitle>
              <AlertDescription>
                The control plane was started without APP_MASTER_KEY set, so it
                cannot encrypt or store bucket credentials. Set APP_MASTER_KEY
                and restart the control plane to enable this.
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
                <CloudArrowUpIcon className="size-4 text-muted-foreground" />
                Connect bucket
              </DialogTitle>
              <DialogDescription>
                An S3-compatible bucket to use as a backup destination for
                managed databases.
              </DialogDescription>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                void onSubmit(e)
              }}
              className="space-y-4"
            >
              <Field>
                <FieldLabel htmlFor="backup-target-name">Name</FieldLabel>
                <Input
                  id="backup-target-name"
                  placeholder="e.g. primary-backups"
                  {...register('name')}
                />
                <FieldError errors={[formState.errors.name]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="backup-target-provider">
                  Provider
                </FieldLabel>
                <Controller
                  control={control}
                  name="provider"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger
                        id="backup-target-provider"
                        className="w-full"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {(Object.keys(PROVIDER_LABEL) as BackupProvider[]).map(
                          (provider) => (
                            <SelectItem key={provider} value={provider}>
                              {PROVIDER_LABEL[provider]}
                            </SelectItem>
                          ),
                        )}
                      </SelectContent>
                    </Select>
                  )}
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="backup-target-bucket">Bucket</FieldLabel>
                <Input
                  id="backup-target-bucket"
                  placeholder="my-backups-bucket"
                  {...register('bucket')}
                />
                <FieldError errors={[formState.errors.bucket]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="backup-target-region">
                  Region{' '}
                  <span className="text-muted-foreground">(optional)</span>
                </FieldLabel>
                <Input
                  id="backup-target-region"
                  placeholder="us-east-1"
                  {...register('region')}
                />
              </Field>

              {watchedProvider === 'aws' ? null : (
                <Field>
                  <FieldLabel htmlFor="backup-target-endpoint">
                    Endpoint
                  </FieldLabel>
                  <Input
                    id="backup-target-endpoint"
                    placeholder={ENDPOINT_PLACEHOLDER[watchedProvider]}
                    {...register('endpoint')}
                  />
                  <FieldError errors={[formState.errors.endpoint]} />
                </Field>
              )}

              <Field>
                <FieldLabel htmlFor="backup-target-access-key-id">
                  Access key ID
                </FieldLabel>
                <Input
                  id="backup-target-access-key-id"
                  className="font-mono"
                  autoComplete="off"
                  autoCapitalize="off"
                  spellCheck={false}
                  {...register('access_key_id')}
                />
                <FieldError errors={[formState.errors.access_key_id]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="backup-target-secret-access-key">
                  Secret access key
                </FieldLabel>
                <div className="relative">
                  <Input
                    id="backup-target-secret-access-key"
                    type={revealSecret ? 'text' : 'password'}
                    className="pr-9 font-mono"
                    autoComplete="off"
                    {...register('secret_access_key')}
                  />
                  <button
                    type="button"
                    onClick={() => {
                      setRevealSecret((v) => !v)
                    }}
                    aria-label={revealSecret ? 'Hide value' : 'Show value'}
                    aria-pressed={revealSecret}
                    className="absolute inset-y-0 right-0 flex w-9 items-center justify-center text-muted-foreground hover:text-foreground"
                  >
                    {revealSecret ? (
                      <EyeSlashIcon className="size-4" />
                    ) : (
                      <EyeIcon className="size-4" />
                    )}
                  </button>
                </div>
                <FieldError errors={[formState.errors.secret_access_key]} />
              </Field>

              {generalError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>{generalError}</AlertDescription>
                </Alert>
              ) : null}

              <DialogFooter>
                <Button type="submit" disabled={createBackupTarget.isPending}>
                  {createBackupTarget.isPending
                    ? 'Connecting...'
                    : 'Connect bucket'}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
