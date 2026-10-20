import { useState } from 'react'
import {
  BuildingsIcon,
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
import { useCreateOrganization } from '../queries/organizations'

// A single name field, mirroring CreateProjectDialog exactly: an
// organization is purely a label grouping projects, no owner or member
// list of its own (internal/api/organizations.go's own doc comment).
export function CreateOrganizationDialog() {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const createOrganization = useCreateOrganization()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setName('')
      createOrganization.reset()
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) {
      return
    }
    createOrganization.mutate(trimmed, {
      onSuccess: (created) => {
        handleOpenChange(false)
        toast.add({
          title: `Organization "${created.name}" created.`,
          type: 'success',
        })
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" />}>
        <PlusIcon />
        New organization
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <BuildingsIcon className="size-4 text-muted-foreground" />
            New organization
          </DialogTitle>
          <DialogDescription>
            A name to group related projects under. Purely organizational,
            nothing about how they run changes.
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <Field>
            <FieldLabel htmlFor="organization-name">Name</FieldLabel>
            <Input
              id="organization-name"
              placeholder="e.g. Acme Inc"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
              autoFocus
            />
          </Field>

          {createOrganization.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>
                {createOrganization.error.message}
              </AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              type="submit"
              disabled={createOrganization.isPending || name.trim() === ''}
            >
              {createOrganization.isPending
                ? 'Creating...'
                : 'Create organization'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
