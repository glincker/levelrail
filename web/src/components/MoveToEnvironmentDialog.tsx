import { useState } from 'react'
import {
  ArrowsLeftRightIcon,
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
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useSetAppEnvironment } from '../queries/apps'
import { useEnvironmentListOptional } from '../queries/environments'

// No-environment sentinel, mirrors MoveToProjectDialog's own
// NO_PROJECT_VALUE exactly and for the identical reason: base-ui's
// Select can't use an empty string as an item value, but
// PUT .../environment treats an empty environment_id as "no environment".
const NO_ENVIRONMENT_VALUE = '__none__'

// Environments are scoped to a project (internal/api/environments.go),
// so an app with no project assigned has nothing to pick from: the
// dialog says so rather than silently showing an empty list, the same
// "explain why, don't just look broken" reasoning MoveToNodeDialog's
// own empty-state text follows.
export function MoveToEnvironmentDialog({
  appName,
  projectId,
  currentEnvironmentId,
}: {
  appName: string
  projectId?: string
  currentEnvironmentId?: string
}) {
  const [open, setOpen] = useState(false)
  const [targetEnvironmentId, setTargetEnvironmentId] = useState(
    NO_ENVIRONMENT_VALUE,
  )
  const setAppEnvironment = useSetAppEnvironment()
  const environmentList = useEnvironmentListOptional(projectId ?? '')
  const environments = environmentList.data ?? []

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setTargetEnvironmentId(currentEnvironmentId || NO_ENVIRONMENT_VALUE)
    } else {
      setAppEnvironment.reset()
    }
  }

  const resolvedTarget =
    targetEnvironmentId === NO_ENVIRONMENT_VALUE ? '' : targetEnvironmentId
  const isNoop = (currentEnvironmentId ?? '') === resolvedTarget

  function handleMove() {
    setAppEnvironment.mutate(
      { name: appName, environmentId: resolvedTarget },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: `App "${appName}" environment updated.`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsLeftRightIcon className="size-3.5" aria-hidden="true" />
        Change
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Tag &ldquo;{appName}&rdquo; with an environment</DialogTitle>
          <DialogDescription>
            Purely organizational: this doesn&apos;t change how {appName} runs.
          </DialogDescription>
        </DialogHeader>

        {!projectId ? (
          <p className="text-sm text-muted-foreground">
            Assign this app to a project first: environments belong to a
            project.
          </p>
        ) : environments.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No environments yet for this app&apos;s project. Create one from
            the project&apos;s own detail page first.
          </p>
        ) : (
          <div className="space-y-1.5">
            <label
              htmlFor="move-target-environment"
              className="text-sm font-medium text-foreground"
            >
              Environment
            </label>
            <Select
              value={targetEnvironmentId}
              onValueChange={(value) => {
                if (value) {
                  setTargetEnvironmentId(value)
                }
              }}
            >
              <SelectTrigger id="move-target-environment" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_ENVIRONMENT_VALUE}>
                  No environment
                  {currentEnvironmentId ? '' : ' (current)'}
                </SelectItem>
                {environments.map((e) => (
                  <SelectItem key={e.id} value={e.id}>
                    {e.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {setAppEnvironment.isError ? (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {setAppEnvironment.error.message}
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
          <Button
            type="button"
            disabled={
              setAppEnvironment.isPending ||
              !projectId ||
              environments.length === 0 ||
              isNoop
            }
            onClick={handleMove}
          >
            {setAppEnvironment.isPending ? 'Saving...' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
