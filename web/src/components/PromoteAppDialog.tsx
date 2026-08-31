import { useState } from 'react'
import { RocketLaunchIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import { useEnvironmentListOptional } from '../queries/environments'
import { usePromoteApp, usePromotePreview } from '../queries/promote'
import { ApiError } from '../lib/apiError'
import { ProtectedEnvironmentNotice } from './ProtectedEnvironmentNotice'

// PromoteAppDialog is "apps promote" (cmd/levelrail-cli/apps_promote.go)
// and POST/GET /api/v1/apps/{name}/promote[/preview]
// (internal/api/promote.go) surfaced on the dashboard: pick a sibling
// environment in this app's project, see which app there would receive
// this app's current image and what would change, then confirm. Target
// app is auto-discovered when exactly one app in the picked environment
// belongs to the project; the optional field below only matters when
// that's ambiguous (internal/api/promote.go's resolvePromotion), the
// same disambiguation the CLI's own --target flag offers.
export function PromoteAppDialog({
  appName,
  projectId,
}: {
  appName: string
  projectId?: string
}) {
  const [open, setOpen] = useState(false)
  const [environmentId, setEnvironmentId] = useState('')
  const [target, setTarget] = useState('')
  const [ackProtected, setAckProtected] = useState(false)

  const environmentList = useEnvironmentListOptional(projectId ?? '')
  const environments = environmentList.data ?? []
  const preview = usePromotePreview(appName, environmentId, target)
  const promote = usePromoteApp(appName)
  const selectedEnvironment = environments.find((e) => e.id === environmentId)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setEnvironmentId('')
      setTarget('')
      setAckProtected(false)
      promote.reset()
    }
  }

  function handlePromote() {
    promote.mutate(
      { to: environmentId, target, confirm: ackProtected },
      {
        onSuccess: (updated) => {
          setOpen(false)
          toast.add({
            title: `Promoted "${appName}" onto "${updated.name}".`,
            description: `Now running ${updated.image}.`,
            type: 'success',
          })
        },
      },
    )
  }

  const previewErrorMessage =
    preview.error instanceof ApiError ? preview.error.message : undefined
  const isNoop = preview.data ? preview.data.changes.length === 0 : false

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <RocketLaunchIcon className="size-3.5" aria-hidden="true" />
        Promote to...
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Promote &ldquo;{appName}&rdquo;</DialogTitle>
          <DialogDescription>
            Points a sibling app in another environment at this app&apos;s
            current image and redeploys it, the same mechanism a deploy
            trigger uses. Scoped to apps within one project.
          </DialogDescription>
        </DialogHeader>

        {!projectId ? (
          <p className="text-sm text-muted-foreground">
            Assign this app to a project first: promotion moves an image
            between apps in the same project.
          </p>
        ) : environments.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No environments yet for this app&apos;s project. Create one from
            the project&apos;s own detail page first.
          </p>
        ) : (
          <div className="space-y-4">
            <Field>
              <FieldLabel htmlFor="promote-target-environment">
                Environment
              </FieldLabel>
              <Select
                value={environmentId}
                onValueChange={(value) => {
                  setEnvironmentId(value ?? '')
                  setTarget('')
                  setAckProtected(false)
                }}
              >
                <SelectTrigger id="promote-target-environment" className="w-full">
                  <SelectValue placeholder="Choose an environment..." />
                </SelectTrigger>
                <SelectContent>
                  {environments.map((e) => (
                    <SelectItem key={e.id} value={e.id}>
                      {e.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel htmlFor="promote-target-app">
                Target app (optional)
              </FieldLabel>
              <Input
                id="promote-target-app"
                placeholder="auto-detected when only one app is tagged"
                value={target}
                onChange={(e) => {
                  setTarget(e.target.value)
                }}
              />
            </Field>

            {environmentId && preview.isPending ? (
              <p className="text-sm text-muted-foreground">
                Loading preview...
              </p>
            ) : null}

            {environmentId && previewErrorMessage ? (
              <p className="flex items-start gap-1.5 text-sm text-destructive">
                <WarningIcon
                  className="mt-0.5 size-4 shrink-0"
                  aria-hidden="true"
                />
                {previewErrorMessage}
              </p>
            ) : null}

            {preview.data ? (
              <div className="space-y-2 rounded-md border border-border p-3 text-sm">
                <p className="text-muted-foreground">
                  Target: <span className="text-foreground">{preview.data.target_app}</span>
                </p>
                {isNoop ? (
                  <p className="text-muted-foreground">
                    No change: target already runs this image.
                  </p>
                ) : (
                  <p className="flex flex-wrap items-center gap-1.5 font-mono text-xs">
                    <span className="text-muted-foreground">
                      {preview.data.to.image || '(none)'}
                    </span>
                    <span aria-hidden="true">&rarr;</span>
                    <span className="text-foreground">
                      {preview.data.from.image}
                    </span>
                  </p>
                )}
              </div>
            ) : null}

            {selectedEnvironment?.protected ? (
              <ProtectedEnvironmentNotice
                id="promote-ack-protected"
                environmentName={selectedEnvironment.name}
                acknowledged={ackProtected}
                onAcknowledgedChange={setAckProtected}
              />
            ) : null}
          </div>
        )}

        {promote.isError ? (
          <Alert variant="destructive">
            <WarningIcon className="size-4" />
            <AlertDescription>{promote.error.message}</AlertDescription>
          </Alert>
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
          <Button
            type="button"
            disabled={
              !environmentId ||
              !preview.data ||
              isNoop ||
              promote.isPending ||
              (selectedEnvironment?.protected && !ackProtected)
            }
            onClick={handlePromote}
          >
            {promote.isPending ? 'Promoting...' : 'Promote'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
