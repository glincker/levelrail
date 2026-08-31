import { useState } from 'react'
import {
  PlusIcon,
  StackSimpleIcon,
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Field, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateEnvironment } from '../queries/environments'

// A single name field, mirroring CreateProjectDialog: an environment is
// a staging/production-style label scoped to one project, tagged onto
// an app via MoveToEnvironmentDialog. Protected can also be toggled
// later from the environment's own detail page
// (ProtectedEnvironmentToggle), this is just a convenience to set it at
// creation time for an operator who already knows this will be
// production.
export function CreateEnvironmentDialog({ projectId }: { projectId: string }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [protectedFlag, setProtectedFlag] = useState(false)
  const createEnvironment = useCreateEnvironment(projectId)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setName('')
      setProtectedFlag(false)
      createEnvironment.reset()
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) {
      return
    }
    createEnvironment.mutate(
      { name: trimmed, protected: protectedFlag },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: `Environment "${created.name}" created.`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <PlusIcon />
        New environment
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <StackSimpleIcon className="size-4 text-muted-foreground" />
            New environment
          </DialogTitle>
          <DialogDescription>
            A staging/production-style label for this project. Tag an app
            with it from the app&apos;s own Overview page.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <Field>
            <FieldLabel htmlFor="environment-name">Name</FieldLabel>
            <Input
              id="environment-name"
              placeholder="e.g. staging"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
              autoFocus
            />
          </Field>

          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={protectedFlag}
              onCheckedChange={(checked) => {
                setProtectedFlag(checked === true)
              }}
            />
            Protected: require confirmation before a deploy, rollback, or
            promote can target an app tagged with this environment
          </label>

          {createEnvironment.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>
                {createEnvironment.error.message}
              </AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              type="submit"
              disabled={createEnvironment.isPending || name.trim() === ''}
            >
              {createEnvironment.isPending ? 'Creating...' : 'Create environment'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
