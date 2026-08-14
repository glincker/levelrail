import { useState } from 'react'
import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { CreateAppFields } from './CreateAppFields'

// Create-via-dialog flow for a new app (POST /api/v1/apps), following
// CreateTokenDialog/CreateAlertRuleDialog's established shape: a
// trigger, a form inside DialogContent, mutate on submit. Unlike those
// two, the trigger itself is a prop rather than fixed inside this
// component, the two-DialogTriggers-one-dialog-content pattern
// CreateTokenDialog's own doc comment gestures at, just with the
// trigger element supplied by each call site instead of hardcoded.
//
// The field set, validation, and mutation logic all live in
// CreateAppFields now (extracted so CreateResourceWizard's step 2
// "Docker image" path can reuse the exact same code, rather than a
// second copy of the same useForm/zod schema); this component is only
// the Dialog chrome around it. Nothing currently in this codebase
// renders this dialog directly (routes/apps/index.tsx uses
// CreateResourceWizard instead, per the creation-wizard-and-sidebar
// design spec), it's kept as a working standalone entry point rather
// than deleted, since its internals are still live and in active use
// via CreateAppFields.
export function CreateAppDialog({ trigger }: { trigger: React.ReactElement }) {
  const [open, setOpen] = useState(false)

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={trigger} />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <PackageIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            New app
          </DialogTitle>
          <DialogDescription>
            Register an app that already has a built image. Apps deployed from a
            git repo&apos;s app.yaml spec show up automatically instead.
          </DialogDescription>
        </DialogHeader>
        <CreateAppFields
          open={open}
          onCreated={() => {
            setOpen(false)
          }}
        />
      </DialogContent>
    </Dialog>
  )
}
