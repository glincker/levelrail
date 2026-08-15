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
import { useSetAppProject } from '../queries/apps'
import { useSetDatabaseProject } from '../queries/databases'
import { useProjectListOptional } from '../queries/projects'

// No-project sentinel, mirrors MoveToNodeDialog's own LOCAL_NODE_VALUE
// exactly and for the identical reason: base-ui's Select can't use an
// empty string as an item value, but both PUT .../project endpoints
// treat an empty project_id as "no project" (setAppProject/
// setDatabaseProject's own doc comments in queries/apps.ts and
// queries/databases.ts).
const NO_PROJECT_VALUE = '__none__'

type MoveToProjectKind = 'app' | 'database'

// One dialog shared by both the app and database Overview sections,
// the exact same reasoning MoveToNodeDialog's own doc comment gives for
// sharing one component instead of two near-identical ones: only which
// PUT .../project endpoint gets called differs.
export function MoveToProjectDialog({
  kind,
  name,
  currentProjectId,
}: {
  kind: MoveToProjectKind
  name: string
  currentProjectId?: string
}) {
  const [open, setOpen] = useState(false)
  const [targetProjectId, setTargetProjectId] = useState(NO_PROJECT_VALUE)
  const setAppProject = useSetAppProject()
  const setDatabaseProject = useSetDatabaseProject()
  const mutation = kind === 'app' ? setAppProject : setDatabaseProject
  // Optional convenience only, see useProjectListOptional's own doc
  // comment: a failure to list projects must degrade to "no projects
  // available" rather than crash the Overview page this trigger lives
  // on.
  const projectList = useProjectListOptional()
  const projects = projectList.data ?? []

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      // Pre-select the resource's actual current project so the dialog
      // reads as "here's where it is, pick somewhere else", mirroring
      // MoveToNodeDialog's own handleOpenChange.
      setTargetProjectId(currentProjectId || NO_PROJECT_VALUE)
    } else {
      setAppProject.reset()
      setDatabaseProject.reset()
    }
  }

  const resolvedTarget =
    targetProjectId === NO_PROJECT_VALUE ? '' : targetProjectId
  const isNoop = (currentProjectId ?? '') === resolvedTarget

  function handleMove() {
    mutation.mutate(
      { name, projectId: resolvedTarget },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: `${kind === 'app' ? 'App' : 'Database'} "${name}" moved.`,
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
        Move
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Move &ldquo;{name}&rdquo; to a project</DialogTitle>
          <DialogDescription>
            Purely organizational: this doesn&apos;t change how {name} runs or
            who can see it.
          </DialogDescription>
        </DialogHeader>

        {projects.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No projects yet. Create one from the Projects page first.
          </p>
        ) : (
          <div className="space-y-1.5">
            <label
              htmlFor="move-target-project"
              className="text-sm font-medium text-foreground"
            >
              Move to
            </label>
            <Select
              value={targetProjectId}
              onValueChange={(value) => {
                if (value) {
                  setTargetProjectId(value)
                }
              }}
            >
              <SelectTrigger id="move-target-project" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_PROJECT_VALUE}>
                  No project
                  {currentProjectId ? '' : ' (current)'}
                </SelectItem>
                {projects.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {mutation.isError ? (
          <p className="flex items-start gap-1.5 text-sm text-destructive">
            <WarningIcon
              className="mt-0.5 size-4 shrink-0"
              aria-hidden="true"
            />
            {mutation.error.message}
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
            disabled={mutation.isPending || projects.length === 0 || isNoop}
            onClick={handleMove}
          >
            {mutation.isPending ? 'Moving...' : 'Move'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
