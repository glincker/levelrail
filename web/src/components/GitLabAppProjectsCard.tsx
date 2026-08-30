import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  GitlabLogoIcon,
  LockIcon,
  SpinnerIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { EmptyState } from '@/components/ui/empty-state'
import { appListQueryOptions } from '../queries/apps'
import {
  useGitLabAppProjects,
  useGitLabAppStatus,
  useConnectGitLabProjectAsSource,
} from '../queries/gitlabApp'
import type { GitLabAppProject } from '../types/gitlabApp'

// Connected-projects list, shown once the GitLab App is authorized: the
// project picker for "use as source", the GitLab counterpart of
// GitRepoSourcePicker.tsx's repo picker, but reachable from the settings
// page itself rather than only from CreateAppFromGitFields.tsx, since
// this action targets an already-existing app (git_sources.go's
// connectGitSource, the exact same step PUT .../git-source uses).
export function GitLabAppProjectsCard() {
  const { data: status } = useGitLabAppStatus()
  const enabled = status.connected && status.authorized
  const projects = useGitLabAppProjects(enabled)
  const [target, setTarget] = useState<GitLabAppProject | null>(null)

  if (!enabled) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GitlabLogoIcon className="size-4" />
          </div>
          <div>
            <CardTitle>Projects</CardTitle>
            <CardDescription>
              Connect a GitLab project as an app&apos;s git source, with a
              push webhook registered automatically.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {projects.isLoading ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <SpinnerIcon className="size-4 animate-spin" />
            Loading projects...
          </p>
        ) : null}
        {projects.isError ? (
          <div className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-destructive">
            <WarningIcon className="mt-0.5 size-4 shrink-0" />
            <p className="text-sm">{projects.error.message}</p>
          </div>
        ) : null}
        {(projects.data ?? []).map((project) => (
          <div
            key={project.id}
            className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2"
          >
            <div className="min-w-0">
              <div className="flex items-center gap-1.5 truncate text-sm font-medium text-foreground">
                {project.visibility === 'private' ? (
                  <LockIcon className="size-3.5 shrink-0 text-muted-foreground" />
                ) : null}
                <span className="truncate">{project.path_with_namespace}</span>
              </div>
              <p className="truncate font-mono text-xs text-muted-foreground">
                {project.default_branch}
              </p>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="shrink-0"
              onClick={() => setTarget(project)}
            >
              Use as source
            </Button>
          </div>
        ))}
        {!projects.isLoading && !projects.isError && (projects.data ?? []).length === 0 ? (
          <EmptyState
            className="py-12"
            icon={<GitlabLogoIcon className="size-5" />}
            title="No accessible projects"
            description="The authorized GitLab App has no projects it can see yet. Grant it access to a project from GitLab to connect it here."
          />
        ) : null}
      </CardContent>
      <UseAsSourceDialog project={target} onOpenChange={(open) => !open && setTarget(null)} />
    </Card>
  )
}

function UseAsSourceDialog({
  project,
  onOpenChange,
}: {
  project: GitLabAppProject | null
  onOpenChange: (open: boolean) => void
}) {
  const apps = useQuery(appListQueryOptions())
  const useAsSource = useConnectGitLabProjectAsSource()
  const [appName, setAppName] = useState('')
  const [branch, setBranch] = useState('')

  function reset() {
    setAppName('')
    setBranch('')
  }

  function handleSubmit() {
    if (!project || !appName) {
      return
    }
    useAsSource.mutate(
      { projectID: project.id, req: { app_name: appName, branch: branch.trim() || undefined } },
      {
        onSuccess: () => {
          toast.add({
            title: 'Project connected.',
            description: `${project.path_with_namespace} is now ${appName}'s git source.`,
            type: 'success',
          })
          reset()
          onOpenChange(false)
        },
        onError: (error) => {
          toast.add({
            title: 'Could not connect the project.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog
      open={project !== null}
      onOpenChange={(next) => {
        if (!next) {
          reset()
        }
        onOpenChange(next)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Use as source</DialogTitle>
          <DialogDescription>
            Connect <span className="font-mono">{project?.path_with_namespace}</span> as
            an app&apos;s git source. A push webhook is registered on the project
            automatically.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="gl-use-app">App</FieldLabel>
            <Select
              value={appName}
              onValueChange={(value) => {
                if (typeof value === 'string') {
                  setAppName(value)
                }
              }}
            >
              <SelectTrigger id="gl-use-app" className="w-full">
                <SelectValue
                  placeholder={apps.isLoading ? 'Loading apps...' : 'Select an app'}
                />
              </SelectTrigger>
              <SelectContent>
                {(apps.data ?? []).map((app) => (
                  <SelectItem key={app.name} value={app.name}>
                    {app.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="gl-use-branch">Branch (optional)</FieldLabel>
            <Input
              id="gl-use-branch"
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              placeholder={project?.default_branch || 'main'}
              value={branch}
              onChange={(e) => {
                setBranch(e.target.value)
              }}
            />
            <FieldDescription>
              Defaults to the project&apos;s own default branch when left blank.
            </FieldDescription>
          </Field>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              reset()
              onOpenChange(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            disabled={!appName || useAsSource.isPending}
            onClick={handleSubmit}
          >
            {useAsSource.isPending ? 'Connecting...' : 'Connect'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
