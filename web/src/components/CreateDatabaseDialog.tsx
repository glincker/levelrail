import { useState } from 'react'
import { DatabaseIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { CreateDatabaseFields } from './CreateDatabaseFields'

// Create-via-dialog flow for a new database (POST /api/v1/databases),
// following CreateAppDialog's shape almost exactly: a trigger supplied
// by the caller rather than fixed inside this component, mutate on
// submit, navigate to the new resource's detail page on success.
//
// The field set, validation, and mutation logic all live in
// CreateDatabaseFields now (extracted so CreateResourceWizard's step 2
// Postgres/Redis paths can reuse the exact same code, rather than a
// second copy of the same useForm/zod schema); this component is only
// the Dialog chrome around it, with the engine left selectable via
// CreateDatabaseFields's own dropdown since (unlike the wizard's step
// 2) nothing has asked "which engine" yet by the time this dialog
// opens. Nothing currently in this codebase renders this dialog
// directly (routes/databases/index.tsx uses CreateResourceWizard
// instead, per the creation-wizard-and-sidebar design spec), it's kept
// as a working standalone entry point rather than deleted, since its
// internals are still live and in active use via CreateDatabaseFields.
export function CreateDatabaseDialog({
  trigger,
}: {
  trigger: React.ReactElement
}) {
  const [open, setOpen] = useState(false)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <DatabaseIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            New database
          </DialogTitle>
          <DialogDescription>
            Register a managed Postgres or Redis database. The reconciler
            provisions it on the target node.
          </DialogDescription>
        </DialogHeader>
        <CreateDatabaseFields
          open={open}
          onCreated={() => {
            setOpen(false)
          }}
        />
      </DialogContent>
    </Dialog>
  )
}
