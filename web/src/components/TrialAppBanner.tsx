import { useState } from 'react'
import { FlaskIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { DeleteAppDialog } from './DeleteAppDialog'

// Surfaced at the top of every section of a service deployed through
// the template catalog's one-click "Deploy now" path
// (store.DesiredService.IsTrial, internal/api/service_templates.go's
// handleDeployServiceTemplateNow): makes it obvious this instance is
// throwaway, not a real deploy an operator forgot about. "Stop & delete"
// reuses DeleteAppDialog as-is, the same full teardown (desired state,
// containers, orphaned app row) any other app delete already performs:
// this is not a second, lighter-weight delete path, only a more
// prominent entry point into the existing one.
export function TrialAppBanner({ name }: { name: string }) {
  const [deleteOpen, setDeleteOpen] = useState(false)

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-violet-200 bg-violet-50 p-3 text-violet-900 sm:flex-row sm:items-center sm:justify-between dark:border-violet-900/50 dark:bg-violet-900/20 dark:text-violet-200">
      <div className="flex items-start gap-2">
        <FlaskIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <div className="text-sm">
          <p className="font-medium">Trial deployment</p>
          <p className="text-violet-800/90 dark:text-violet-200/80">
            Deployed to try the template. Stop and delete it once you&apos;re
            done, or it keeps running like any other app.
          </p>
        </div>
      </div>
      <Button
        type="button"
        size="sm"
        variant="destructive"
        className="shrink-0"
        onClick={() => setDeleteOpen(true)}
      >
        <TrashIcon className="size-3.5" aria-hidden="true" />
        Stop & delete
      </Button>
      <DeleteAppDialog
        name={name}
        control={{
          open: deleteOpen,
          onOpenChange: setDeleteOpen,
          hideTrigger: true,
        }}
      />
    </div>
  )
}
