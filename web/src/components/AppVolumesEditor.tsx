import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  TrashIcon,
  HardDrivesIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useSetAppVolumes } from '../queries/appVolumes'
import { useRestartRequiredToast } from '../hooks/useRestartRequiredToast'
import type { AppVolume } from '../types/appDetail'

// Mirrors internal/api/apps_volumes_attach.go's own volumeNameRe: lowercase
// alphanumeric and hyphens, starting with a letter, the same convention
// internal/spec's nameLike enforces for an app.yaml-declared volume.
const volumeNameRe = /^[a-z][a-z0-9-]*$/

const newVolumeSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Name is required')
    .regex(
      volumeNameRe,
      'Lowercase letters, numbers, and hyphens, starting with a letter',
    ),
  path: z
    .string()
    .trim()
    .min(2, 'Mount path is required')
    .regex(/^\//, 'Must be an absolute path, starting with /'),
})

type NewVolumeFormValues = z.infer<typeof newVolumeSchema>

// Detaching stops the container from mounting this path on its next
// restart, so a confirm step matters even though the volume itself
// (and its data) is untouched in Docker.
function DetachVolumeButton({
  volume,
  disabled,
  onConfirm,
}: {
  volume: AppVolume
  disabled: boolean
  onConfirm: () => void
}) {
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={disabled}
            aria-label={`Detach ${volume.name}`}
          />
        }
      >
        <TrashIcon
          className="size-4 text-muted-foreground"
          aria-hidden="true"
        />
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Detach &ldquo;{volume.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            {volume.container_path} stops being mounted on this app&apos;s next
            redeploy or restart. The volume itself is not deleted and can be
            reattached later.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => setOpen(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={() => {
              setOpen(false)
              onConfirm()
            }}
          >
            Detach
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Attach (or remove) a named Docker volume on this service outside a
// redeploy, via PUT /api/v1/apps/{name}/volumes (queries/appVolumes.ts).
// Previously the only way to declare a volume was hand-editing app.yaml
// and redeploying; this is the form-based counterpart EnvEditor already
// has for env vars, same full-replace-the-list shape as
// PreviewEnvOverridesEditor's own add/remove rows.
//
// A size field is deliberately omitted: Docker's local volume driver has
// no quota concept, so a "size" input here would be decorative, not
// enforced, and resize is not offered for the same reason (see
// internal/docker's volume helpers, which only ever report observed disk
// usage, never a settable limit).
export function AppVolumesEditor({
  appName,
  volumes,
}: {
  appName: string
  volumes: AppVolume[] | undefined
}) {
  const setVolumes = useSetAppVolumes(appName)
  const notifyRestartRequired = useRestartRequiredToast()
  const { register, handleSubmit, formState, reset, setError } =
    useForm<NewVolumeFormValues>({
      resolver: zodResolver(newVolumeSchema),
      defaultValues: { name: '', path: '' },
    })

  const current = volumes ?? []

  const onSubmit = handleSubmit((values) => {
    const name = values.name.trim()
    const path = values.path.trim()
    if (current.some((v) => v.name === name)) {
      setError('name', { message: `A volume named "${name}" already exists` })
      return
    }
    if (current.some((v) => v.container_path === path)) {
      setError('path', { message: `"${path}" is already mounted` })
      return
    }
    setVolumes.mutate([...current, { name, container_path: path }], {
      onSuccess: () => {
        reset({ name: '', path: '' })
        notifyRestartRequired(appName, `Volume "${name}" attached.`)
      },
    })
  })

  const removeVolume = (name: string) => {
    setVolumes.mutate(
      current.filter((v) => v.name !== name),
      {
        onSuccess: () => {
          notifyRestartRequired(appName, `Volume "${name}" detached.`)
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HardDrivesIcon className="size-4" aria-hidden="true" />
          Volumes
        </CardTitle>
        <CardDescription>
          Named Docker volumes mounted into this service&apos;s container. Takes
          effect the next time this app redeploys or restarts.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {current.length > 0 ? (
          <ul className="mb-4 space-y-1 rounded-md border border-border p-2">
            {current.map((v) => (
              <li
                key={v.name}
                className="flex items-center justify-between gap-2 rounded px-2 py-1 text-sm"
              >
                <span className="font-mono">
                  {v.name}
                  <span className="ml-2 text-muted-foreground">
                    {v.container_path}
                  </span>
                </span>
                <DetachVolumeButton
                  volume={v}
                  disabled={setVolumes.isPending}
                  onConfirm={() => {
                    removeVolume(v.name)
                  }}
                />
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
              <FieldLabel htmlFor="new-volume-name">Volume name</FieldLabel>
              <Input
                id="new-volume-name"
                className="font-mono"
                placeholder="data"
                autoComplete="off"
                autoCapitalize="off"
                spellCheck={false}
                {...register('name')}
              />
              <FieldError
                errors={
                  formState.errors.name ? [formState.errors.name] : undefined
                }
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="new-volume-path">Mount path</FieldLabel>
              <Input
                id="new-volume-path"
                className="font-mono"
                placeholder="/data"
                autoComplete="off"
                spellCheck={false}
                {...register('path')}
              />
              <FieldError
                errors={
                  formState.errors.path ? [formState.errors.path] : undefined
                }
              />
            </Field>
          </FieldGroup>
          <div className="mt-3">
            <Button type="submit" size="sm" disabled={setVolumes.isPending}>
              {setVolumes.isPending ? 'Attaching...' : 'Attach volume'}
            </Button>
          </div>
          {setVolumes.isError ? (
            <Alert variant="destructive" className="mt-3">
              <AlertDescription>{setVolumes.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
