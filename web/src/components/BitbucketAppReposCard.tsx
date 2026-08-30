import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  GitBranchIcon,
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
import { appListQueryOptions } from '../queries/apps'
import {
  useBitbucketAppRepos,
  useBitbucketAppStatus,
  useConnectBitbucketRepoAsSource,
} from '../queries/bitbucketApp'
import type { BitbucketAppRepo } from '../types/bitbucketApp'

// Connected-repos list, shown once the Bitbucket App is authorized: the
// repo picker for "use as source", mirroring GitLabAppProjectsCard.tsx
// exactly except keyed by full_name ("workspace/repo_slug") rather than
// a numeric project ID, since Bitbucket has no such ID for a
// repository.
export function BitbucketAppReposCard() {
  const { data: status } = useBitbucketAppStatus()
  const enabled = status.connected && status.authorized
  const repos = useBitbucketAppRepos(enabled)
  const [target, setTarget] = useState<BitbucketAppRepo | null>(null)

  if (!enabled) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GitBranchIcon className="size-4" />
          </div>
          <div>
            <CardTitle>Repositories</CardTitle>
            <CardDescription>
              Connect a Bitbucket repository as an app&apos;s git source, with
              a push webhook registered automatically.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {repos.isLoading ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <SpinnerIcon className="size-4 animate-spin" />
            Loading repositories...
          </p>
        ) : null}
        {repos.isError ? (
          <div className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 p-2.5 text-destructive">
            <WarningIcon className="mt-0.5 size-4 shrink-0" />
            <p className="text-sm">{repos.error.message}</p>
          </div>
        ) : null}
        {(repos.data ?? []).map((repo) => (
          <div
            key={repo.full_name}
            className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2"
          >
            <div className="min-w-0">
              <div className="flex items-center gap-1.5 truncate text-sm font-medium text-foreground">
                {repo.private ? (
                  <LockIcon className="size-3.5 shrink-0 text-muted-foreground" />
                ) : null}
                <span className="truncate">{repo.full_name}</span>
              </div>
              <p className="truncate font-mono text-xs text-muted-foreground">
                {repo.default_branch}
              </p>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="shrink-0"
              onClick={() => setTarget(repo)}
            >
              Use as source
            </Button>
          </div>
        ))}
        {!repos.isLoading && !repos.isError && (repos.data ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">No accessible repositories found.</p>
        ) : null}
      </CardContent>
      <UseAsSourceDialog repo={target} onOpenChange={(open) => !open && setTarget(null)} />
    </Card>
  )
}

function UseAsSourceDialog({
  repo,
  onOpenChange,
}: Readonly<{
  repo: BitbucketAppRepo | null
  onOpenChange: (open: boolean) => void
}>) {
  const apps = useQuery(appListQueryOptions())
  const useAsSource = useConnectBitbucketRepoAsSource()
  const [appName, setAppName] = useState('')
  const [branch, setBranch] = useState('')

  function reset() {
    setAppName('')
    setBranch('')
  }

  function handleSubmit() {
    if (!repo || !appName) {
      return
    }
    const [workspace, repoSlug] = repo.full_name.split('/')
    if (!workspace || !repoSlug) {
      return
    }
    useAsSource.mutate(
      {
        workspace,
        repoSlug,
        req: { app_name: appName, branch: branch.trim() || undefined },
      },
      {
        onSuccess: () => {
          toast.add({
            title: 'Repository connected.',
            description: `${repo.full_name} is now ${appName}'s git source.`,
            type: 'success',
          })
          reset()
          onOpenChange(false)
        },
        onError: (error) => {
          toast.add({
            title: 'Could not connect the repository.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <Dialog
      open={repo !== null}
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
            Connect <span className="font-mono">{repo?.full_name}</span> as an
            app&apos;s git source. A push webhook is registered on the
            repository automatically.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field>
            <FieldLabel htmlFor="bb-use-app">App</FieldLabel>
            <Select
              value={appName}
              onValueChange={(value) => {
                if (typeof value === 'string') {
                  setAppName(value)
                }
              }}
            >
              <SelectTrigger id="bb-use-app" className="w-full">
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
            <FieldLabel htmlFor="bb-use-branch">Branch (optional)</FieldLabel>
            <Input
              id="bb-use-branch"
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              placeholder={repo?.default_branch || 'main'}
              value={branch}
              onChange={(e) => {
                setBranch(e.target.value)
              }}
            />
            <FieldDescription>
              Defaults to the repository&apos;s own default branch when left
              blank.
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
