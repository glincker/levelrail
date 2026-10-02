import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  EyeIcon,
  EyeSlashIcon,
  HardDrivesIcon,
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
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import { useCreateNetworkShare } from '../queries/networkShares'
import type { NetworkShareProtocol } from '../types/networkShare'

// Mirrors validateNetworkShareFields (internal/api/network_shares.go):
// name/protocol/host/remote_path are always required, username/password
// only required for a cifs share. Client-side fast feedback only, same
// reasoning createRegistryCredentialSchema's own comment gives.
const createNetworkShareSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    protocol: z.enum(['nfs', 'cifs']),
    host: z.string().trim().min(1, 'Host is required'),
    remote_path: z.string().trim().min(1, 'Remote path is required'),
    mount_options: z.string(),
    username: z.string(),
    password: z.string(),
  })
  .superRefine((values, ctx) => {
    if (values.protocol !== 'cifs') {
      return
    }
    if (values.username.trim() === '') {
      ctx.addIssue({
        code: 'custom',
        path: ['username'],
        message: 'Username is required for a cifs share',
      })
    }
    if (values.password === '') {
      ctx.addIssue({
        code: 'custom',
        path: ['password'],
        message: 'Password is required for a cifs share',
      })
    }
  })

type CreateNetworkShareFormValues = z.infer<typeof createNetworkShareSchema>

const defaultValues: CreateNetworkShareFormValues = {
  name: '',
  protocol: 'nfs',
  host: '',
  remote_path: '',
  mount_options: '',
  username: '',
  password: '',
}

// Adds an NFS export or CIFS/SMB share, mounted as a Docker `local`-
// driver volume via Docker's own built-in NFS/CIFS passthrough (no
// shelling out to the docker CLI). POST /api/v1/network-shares requires
// write:sensitive for a cifs share, the same ability tier
// CreateRegistryCredentialDialog's own doc comment explains for the
// identical reason (a live credential in the request body).
export function CreateNetworkShareDialog() {
  const [open, setOpen] = useState(false)
  const [revealPassword, setRevealPassword] = useState(false)
  const createShare = useCreateNetworkShare()
  const { register, handleSubmit, formState, reset, watch } =
    useForm<CreateNetworkShareFormValues>({
      resolver: zodResolver(createNetworkShareSchema),
      defaultValues,
    })

  const protocol: NetworkShareProtocol = watch('protocol')
  const isCifs = protocol === 'cifs'

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      setRevealPassword(false)
      createShare.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    createShare.mutate(
      {
        name: values.name.trim(),
        protocol: values.protocol,
        host: values.host.trim(),
        remote_path: values.remote_path.trim(),
        mount_options: values.mount_options.trim() || undefined,
        username: isCifs ? values.username.trim() : undefined,
        password: isCifs ? values.password : undefined,
      },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: `Network share "${created.name}" added.`,
            type: 'success',
          })
        },
      },
    )
  })

  const notConfigured =
    createShare.isError &&
    createShare.error instanceof ApiError &&
    createShare.error.status === 501

  const generalError =
    createShare.isError && !notConfigured ? createShare.error.message : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Add network share</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {notConfigured ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <HardDrivesIcon className="size-4 text-muted-foreground" />
                Add network share
              </DialogTitle>
            </DialogHeader>
            <Alert variant="destructive">
              <AlertTitle>
                CIFS/SMB shares are not configured on this server
              </AlertTitle>
              <AlertDescription>
                The control plane was started without APP_MASTER_KEY set, so it
                cannot encrypt or store a CIFS password. Set APP_MASTER_KEY and
                restart the control plane to enable this, or add an NFS export
                instead (no password required).
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
                <HardDrivesIcon className="size-4 text-muted-foreground" />
                Add network share
              </DialogTitle>
              <DialogDescription>
                An NFS export or CIFS/SMB share, mounted as a Docker volume
                using the local driver's built-in network passthrough.
              </DialogDescription>
            </DialogHeader>
            <form
              onSubmit={(e) => {
                void onSubmit(e)
              }}
              className="space-y-4"
            >
              <Field>
                <FieldLabel htmlFor="network-share-name">Name</FieldLabel>
                <Input
                  id="network-share-name"
                  placeholder="e.g. media-nas"
                  {...register('name')}
                />
                <FieldError errors={[formState.errors.name]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="network-share-protocol">
                  Protocol
                </FieldLabel>
                <Select
                  value={protocol}
                  onValueChange={(value) => {
                    if (value === 'nfs' || value === 'cifs') {
                      reset(
                        { ...defaultValues, protocol: value },
                        { keepDirty: false },
                      )
                    }
                  }}
                >
                  <SelectTrigger id="network-share-protocol" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="nfs">NFS</SelectItem>
                    <SelectItem value="cifs">CIFS / SMB</SelectItem>
                  </SelectContent>
                </Select>
              </Field>

              <Field>
                <FieldLabel htmlFor="network-share-host">Host</FieldLabel>
                <Input
                  id="network-share-host"
                  className="font-mono"
                  placeholder="nas.lan or 192.168.1.50"
                  {...register('host')}
                />
                <FieldError errors={[formState.errors.host]} />
              </Field>

              <Field>
                <FieldLabel htmlFor="network-share-remote-path">
                  {isCifs ? 'Share name' : 'Export path'}
                </FieldLabel>
                <Input
                  id="network-share-remote-path"
                  className="font-mono"
                  placeholder={isCifs ? '/backups' : '/export/media'}
                  {...register('remote_path')}
                />
                <FieldError errors={[formState.errors.remote_path]} />
              </Field>

              {isCifs ? (
                <>
                  <Field>
                    <FieldLabel htmlFor="network-share-username">
                      Username
                    </FieldLabel>
                    <Input
                      id="network-share-username"
                      autoComplete="off"
                      autoCapitalize="off"
                      spellCheck={false}
                      {...register('username')}
                    />
                    <FieldError errors={[formState.errors.username]} />
                  </Field>

                  <Field>
                    <FieldLabel htmlFor="network-share-password">
                      Password
                    </FieldLabel>
                    <div className="relative">
                      <Input
                        id="network-share-password"
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
                        aria-label={
                          revealPassword ? 'Hide value' : 'Show value'
                        }
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
                </>
              ) : null}

              <Field>
                <FieldLabel htmlFor="network-share-mount-options">
                  Mount options{' '}
                  <span className="text-muted-foreground">(optional)</span>
                </FieldLabel>
                <Input
                  id="network-share-mount-options"
                  className="font-mono"
                  placeholder={isCifs ? 'vers=3.0' : 'nfsvers=4,ro'}
                  {...register('mount_options')}
                />
                <FieldDescription>
                  Extra comma-separated -o options passed straight through to
                  Docker's local driver.
                </FieldDescription>
              </Field>

              {generalError ? (
                <Alert variant="destructive">
                  <WarningIcon />
                  <AlertDescription>{generalError}</AlertDescription>
                </Alert>
              ) : null}

              <DialogFooter>
                <Button type="submit" disabled={createShare.isPending}>
                  {createShare.isPending ? 'Adding...' : 'Add network share'}
                </Button>
              </DialogFooter>
            </form>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
