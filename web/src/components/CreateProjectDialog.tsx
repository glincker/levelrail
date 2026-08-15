import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import {
  FolderIcon,
  PlusIcon,
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
import { Input } from '@/components/ui/input'
import { Field, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateProject } from '../queries/projects'

// A single name field, nothing else: a project is purely a label
// (internal/api/projects.go's own package doc comment), so there is
// no engine picker, no node placement, no multi-step wizard the way
// CreateResourceWizard needs for apps/databases. Resets on close via
// onOpenChange, the same pattern AddNodeDialog/MoveToNodeDialog/
// MoveToProjectDialog already use for plain useState-backed dialogs
// (react-hook-form-backed forms like CreateAppFields instead reset via
// an effect keyed on the owning dialog's open prop, since reset() there
// isn't a raw setState call).
export function CreateProjectDialog() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const navigate = useNavigate()
  const createProject = useCreateProject()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setName('')
      createProject.reset()
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) {
      return
    }
    createProject.mutate(trimmed, {
      onSuccess: (created) => {
        handleOpenChange(false)
        toast.add({
          title: `Project "${created.name}" created.`,
          type: 'success',
        })
        void navigate({ to: '/projects/$id', params: { id: created.id } })
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" />}>
        <PlusIcon />
        New project
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FolderIcon className="size-4 text-muted-foreground" />
            New project
          </DialogTitle>
          <DialogDescription>
            A name to group related apps and databases under. Purely
            organizational, nothing about how they run changes.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <Field>
            <FieldLabel htmlFor="project-name">Name</FieldLabel>
            <Input
              id="project-name"
              placeholder="e.g. my-saas"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
              autoFocus
            />
          </Field>

          {createProject.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{createProject.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              type="submit"
              disabled={createProject.isPending || name.trim() === ''}
            >
              {createProject.isPending ? 'Creating...' : 'Create project'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
