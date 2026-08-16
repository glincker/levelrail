import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { GithubLogoIcon } from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { GitHubAppConnectionCard } from '../../components/GitHubAppConnectionCard'
import { githubAppStatusQueryOptions } from '../../queries/githubApp'
import { ingressSettingsQueryOptions } from '../../queries/domains'

// Account-level, not scoped to one app: lives under routes/settings/
// next to backup-targets.tsx and tokens.tsx, the same reasoning those
// files' own header comments give for their placement. Typed loader
// primes both queries this page needs (connection status, ingress
// settings for the primary-domain check) before render, the same
// pattern backup-targets.tsx already established.
//
// This page covers only what internal/api's GitHub App routes actually
// do: connecting/viewing status/disconnecting, and (once connected) a
// repo/branch picker feeding CreateAppFromGitFields.tsx. It does not
// cover auto-deploy-on-push or PR preview environments: neither exists
// on the backend yet, both are explicit, separate follow-up work.
export const Route = createFileRoute('/settings/github-app')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(githubAppStatusQueryOptions()),
      queryClient.ensureQueryData(ingressSettingsQueryOptions()),
    ]),
  component: GitHubAppSettingsPage,
})

function GitHubAppSettingsPage() {
  // Primed by the loader; read here too so this route re-renders if a
  // mutation elsewhere invalidates the same query key, same convention
  // BackupTargetsPage already follows.
  useSuspenseQuery(githubAppStatusQueryOptions())
  const navigate = useNavigate()

  // GitHub's post-install redirect (setup_url,
  // internal/api's handleGitHubAppInstalled) lands back here with
  // ?github_app=installed on success: a one-time toast, then the query
  // param is stripped so a page refresh doesn't re-show it.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('github_app') === 'installed') {
      toast.add({
        title: 'GitHub App installed.',
        description: 'Repositories are now available to pick from.',
        type: 'success',
      })
      void navigate({ to: '/settings/github-app', replace: true })
    }
    // Only reacting to the initial redirect landing, not to navigate's
    // own identity churn on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <GithubLogoIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">GitHub App</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect a GitHub App for private-repository access and
            installation-based repo browsing when creating an app from git.
          </p>
        </div>
      </div>
      <GitHubAppConnectionCard />
    </div>
  )
}
