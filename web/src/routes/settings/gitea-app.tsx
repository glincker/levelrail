import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { TeaBagIcon } from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { GiteaAppConnectionCard } from '../../components/GiteaAppConnectionCard'
import { GiteaAppReposCard } from '../../components/GiteaAppReposCard'
import { giteaAppStatusQueryOptions } from '../../queries/giteaApp'
import { PageSpinner } from '@/components/ui/page-spinner'

// Account-level, mirroring routes/settings/bitbucket-app.tsx's own
// structure and placement.
export const Route = createFileRoute('/settings/gitea-app')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(giteaAppStatusQueryOptions()),
  component: GiteaAppSettingsPage,
  pendingComponent: PageSpinner,
})

function GiteaAppSettingsPage() {
  useSuspenseQuery(giteaAppStatusQueryOptions())
  const navigate = useNavigate()

  // Gitea's OAuth callback redirect lands back here with
  // ?gitea_app=connected on success, mirroring the gitlab-app/
  // bitbucket-app routes' own one-time toast then query-param strip.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('gitea_app') === 'connected') {
      toast.add({
        title: 'Gitea App authorized.',
        description: 'Repositories are now available to pick from.',
        type: 'success',
      })
      void navigate({ to: '/settings/gitea-app', replace: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <TeaBagIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Gitea App</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect a self-hosted Gitea OAuth2 application for repository
            browsing and webhook-driven deploys.
          </p>
        </div>
      </div>
      <GiteaAppConnectionCard />
      <GiteaAppReposCard />
    </div>
  )
}
