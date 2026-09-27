import { useState } from 'react'
import type { DialogControl } from './dialogControl'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useNavigate } from '@tanstack/react-router'
import { CopyIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCloneApp, useClonePreview } from '../queries/apps'
import type { ClonePreview } from '../queries/apps'
import { Checkbox } from '@/components/ui/checkbox'

// Mirrors CreateAppFields' name validation exactly (non-empty, trimmed):
// a sanity check for fast feedback, not a substitute for the server's
// own validation. The real conflict check (a name that already exists,
// including cloning onto the source's own name) only ever happens
// server-side, in handleCloneApp, and its 409 surfaces below the same
// way DeleteAppDialog/MoveToProjectDialog show their own mutation
// errors.
function CloneOptions({
  preview,
  copySecrets,
  onCopySecrets,
  copyDomains,
  onCopyDomains,
}: {
  preview: ClonePreview | undefined
  copySecrets: boolean
  onCopySecrets: (next: boolean) => void
  copyDomains: boolean
  onCopyDomains: (next: boolean) => void
}) {
  if (!preview) return null
  return (
    <div className="space-y-3 rounded-md border border-border p-3 text-xs">
      <div className="space-y-1">
        <p className="font-medium text-foreground">Copied</p>
        <p className="text-muted-foreground">{preview.will_copy.join('; ')}</p>
      </div>
      <div className="space-y-1">
        <p className="font-medium text-foreground">Not copied</p>
        <p className="text-muted-foreground">
          {preview.will_not_copy.join('; ')}
        </p>
      </div>
      {preview.secret_names.length > 0 ? (
        <label className="flex items-start gap-2 text-sm">
          <Checkbox checked={copySecrets} onCheckedChange={onCopySecrets} />
          <span>
            Copy {preview.secret_names.length} secret value
            {preview.secret_names.length === 1 ? '' : 's'}
            <span className="block text-xs text-muted-foreground">
              Re-encrypted for the clone, never shown. Off by default: names are
              copied, values stay empty.
            </span>
          </span>
        </label>
      ) : null}
      {preview.source_domains.length > 0 ? (
        <label className="flex items-start gap-2 text-sm">
          <Checkbox checked={copyDomains} onCheckedChange={onCopyDomains} />
          <span>
            Derive new domains from {preview.source_domains.join(', ')}
            <span className="block text-xs text-muted-foreground">
              Adds the new app name to the first label. Off by default: the
              clone starts with no domains.
            </span>
          </span>
        </label>
      ) : null}
    </div>
  )
}

const cloneAppSchema = z.object({
  newName: z.string().trim().min(1, 'Name is required'),
})
type CloneAppFormValues = z.infer<typeof cloneAppSchema>

// One app's clone action, split out of routes/apps/$name.tsx's header
// the same way RestartAppButton/DeleteAppDialog are. POST
// /api/v1/apps/{name}/clone (internal/api/apps_clone.go's handleCloneApp)
// duplicates image/port/env/resource limits/health checks/deploy
// strategy/replicas/project into a brand new app; it deliberately does
// not carry over domains, node placement, or secret values, see that
// handler's own doc comment for the full reasoning. This dialog's
// description names that boundary up front so cloning an app with a
// live domain or secrets doesn't read as silently incomplete.
//
// On success, navigates to the new app's own detail page, the same
// success shape CreateAppFields already establishes for a brand-new
// app.
export function CloneAppDialog({
  name,
  control,
}: {
  name: string
  control?: DialogControl
}) {
  const [internalOpen, setInternalOpen] = useState(false)
  const open = control?.open ?? internalOpen
  const setOpen = (next: boolean) => {
    setInternalOpen(next)
    control?.onOpenChange?.(next)
  }
  const navigate = useNavigate()
  const cloneApp = useCloneApp()
  const preview = useClonePreview(name, open)
  const [copySecrets, setCopySecrets] = useState(false)
  const [copyDomains, setCopyDomains] = useState(false)
  const { register, handleSubmit, formState, reset } =
    useForm<CloneAppFormValues>({
      resolver: zodResolver(cloneAppSchema),
      defaultValues: { newName: '' },
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset({ newName: '' })
      setCopySecrets(false)
      setCopyDomains(false)
      cloneApp.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    cloneApp.mutate(
      {
        name,
        newName: values.newName.trim(),
        options: {
          copySecrets,
          domainSuffix: copyDomains ? values.newName.trim() : undefined,
        },
      },
      {
        onSuccess: (cloned) => {
          setOpen(false)
          toast.add({
            title: `App "${name}" cloned to "${cloned.name}".`,
            type: 'success',
          })
          void navigate({
            to: '/apps/$name',
            params: { name: cloned.name },
          })
        },
      },
    )
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      {control?.hideTrigger ? null : (
        <DialogTrigger
          render={<Button type="button" variant="outline" size="sm" />}
        >
          <CopyIcon className="size-3.5" aria-hidden="true" />
          Clone
        </DialogTrigger>
      )}
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Clone &ldquo;{name}&rdquo;</DialogTitle>
          <DialogDescription>
            Copies the app&apos;s configuration into a new app. Cloning never
            starts a deploy, so approvals and freeze windows apply when you
            first deploy the clone.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="clone-new-name">New app name</FieldLabel>
            <Input
              id="clone-new-name"
              placeholder="e.g. web-staging"
              {...register('newName')}
            />
            <FieldError errors={[formState.errors.newName]} />
          </Field>

          <CloneOptions
            preview={preview.data}
            copySecrets={copySecrets}
            onCopySecrets={setCopySecrets}
            copyDomains={copyDomains}
            onCopyDomains={setCopyDomains}
          />

          {cloneApp.isError ? (
            <p className="flex items-start gap-1.5 text-sm text-destructive">
              <WarningIcon
                className="mt-0.5 size-4 shrink-0"
                aria-hidden="true"
              />
              {cloneApp.error.message}
            </p>
          ) : null}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                handleOpenChange(false)
              }}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={cloneApp.isPending}>
              {cloneApp.isPending ? 'Cloning...' : 'Clone app'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
