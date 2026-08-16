import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { GithubLogoIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  useGitHubAppBranches,
  useGitHubAppRepos,
  useGitHubAppStatus,
} from '../queries/githubApp'

// A repo+branch picker fed by the connected GitHub App installation, an
// alternative to hand-typing a repository URL in
// CreateAppFromGitFields.tsx. Always rendered: when no App connection
// exists yet it renders a short CTA pointing at the connect flow instead
// of the picker, rather than disappearing and leaving the git fields
// below with no explanation for why there's no picker.
//
// Selecting a private repository here fills in its clone URL, and
// building it (POST /apps/{name}/builds, internal/api/builds.go)
// authenticates the clone with a freshly minted installation token for
// any github.com repo URL when this same App connection exists
// (tokenForRepo, builds.go), so a private repo picked here actually
// builds.
export function GitHubAppRepoPicker({
  onSelect,
}: {
  /** Called with (repoUrl, ref) once both a repo and branch are picked. */
  onSelect: (repoUrl: string, ref: string) => void
}) {
  const { data: status } = useGitHubAppStatus()
  const enabled = status.connected && status.installed
  const repos = useGitHubAppRepos(enabled)
  const [selectedRepo, setSelectedRepo] = useState('')
  const branches = useGitHubAppBranches(
    selectedRepo.split('/')[0] ?? '',
    selectedRepo.split('/')[1] ?? '',
    selectedRepo !== '',
  )

  const repoByFullName = useMemo(() => {
    const map = new Map<string, { cloneUrl: string; defaultBranch: string }>()
    for (const repo of repos.data ?? []) {
      map.set(repo.full_name, {
        cloneUrl: repo.clone_url,
        defaultBranch: repo.default_branch,
      })
    }
    return map
  }, [repos.data])

  if (!enabled) {
    return (
      <div className="flex items-start gap-2 rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground">
        <GithubLogoIcon className="mt-0.5 size-4 shrink-0" />
        <p>
          Paste a public repository URL below, or{' '}
          <Link
            to="/settings/github-app"
            className="text-primary underline underline-offset-2"
          >
            connect GitHub
          </Link>{' '}
          for private-repo access and a repo picker here.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-lg border border-dashed border-border p-3">
      <div className="flex items-center gap-2 text-sm font-medium text-foreground">
        <GithubLogoIcon className="size-4" />
        Pick from your connected GitHub App
      </div>

      <Field>
        <FieldLabel htmlFor="github-app-repo">Repository</FieldLabel>
        <Select
          value={selectedRepo}
          onValueChange={(value) => {
            if (typeof value === 'string') {
              setSelectedRepo(value)
            }
          }}
        >
          <SelectTrigger id="github-app-repo" className="w-full">
            <SelectValue
              placeholder={
                repos.isLoading
                  ? 'Loading repositories...'
                  : 'Select a repository'
              }
            />
          </SelectTrigger>
          <SelectContent>
            {(repos.data ?? []).map((repo) => (
              <SelectItem key={repo.full_name} value={repo.full_name}>
                {repo.full_name}
                {repo.private ? ' (private)' : ''}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {repos.isError ? (
          <p className="text-sm text-destructive">{repos.error.message}</p>
        ) : null}
      </Field>

      {selectedRepo ? (
        <Field>
          <FieldLabel htmlFor="github-app-branch">Branch</FieldLabel>
          <Select
            onValueChange={(ref) => {
              if (typeof ref !== 'string') {
                return
              }
              const repo = repoByFullName.get(selectedRepo)
              if (repo) {
                onSelect(repo.cloneUrl, ref)
              }
            }}
          >
            <SelectTrigger id="github-app-branch" className="w-full">
              <SelectValue
                placeholder={
                  branches.isLoading ? 'Loading branches...' : 'Select a branch'
                }
              />
            </SelectTrigger>
            <SelectContent>
              {(branches.data ?? []).map((branch) => (
                <SelectItem key={branch.name} value={branch.name}>
                  {branch.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {branches.isError ? (
            <p className="text-sm text-destructive">{branches.error.message}</p>
          ) : null}
        </Field>
      ) : null}
    </div>
  )
}
