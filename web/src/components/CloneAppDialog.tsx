import { useState } from 'react'
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
import { useCloneApp } from '../queries/apps'

// Mirrors CreateAppFields' name validation exactly (non-empty, trimmed):
// a sanity check for fast feedback, not a substitute for the server's
// own validation. The real conflict check (a name that already exists,
// including cloning onto the source's own name) only ever happens
// server-side, in handleCloneApp, and its 409 surfaces below the same
// way DeleteAppDialog/MoveToProjectDialog show their own mutation
// errors.
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
export function CloneAppDialog({ name }: { name: string }) {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const cloneApp = useCloneApp()
  const { register, handleSubmit, formState, reset } =
    useForm<CloneAppFormValues>({
      resolver: zodResolver(cloneAppSchema),
      defaultValues: { newName: '' },
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset({ newName: '' })
      cloneApp.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    cloneApp.mutate(
      { name, newName: values.newName.trim() },
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
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <CopyIcon className="size-3.5" aria-hidden="true" />
        Clone
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Clone &ldquo;{name}&rdquo;</DialogTitle>
          <DialogDescription>
            Copies image, port, env, resource limits, health checks, deploy
            strategy, replicas, and project into a new app. Domains, node
            placement, and secret values are not copied: connect a domain and
            re-set secrets for the new app separately. Cloning does not
            start a deploy.
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
