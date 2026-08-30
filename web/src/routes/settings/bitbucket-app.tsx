import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { GitBranchIcon } from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { BitbucketAppConnectionCard } from '../../components/BitbucketAppConnectionCard'
import { BitbucketAppReposCard } from '../../components/BitbucketAppReposCard'
import { bitbucketAppStatusQueryOptions } from '../../queries/bitbucketApp'
import { PageSpinner } from '@/components/ui/page-spinner'

// Account-level, mirroring routes/settings/gitlab-app.tsx's own
// structure and placement.
export const Route = createFileRoute('/settings/bitbucket-app')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(bitbucketAppStatusQueryOptions()),
  component: BitbucketAppSettingsPage,
  pendingComponent: PageSpinner,
})

function BitbucketAppSettingsPage() {
  useSuspenseQuery(bitbucketAppStatusQueryOptions())
  const navigate = useNavigate()

  // Bitbucket's OAuth callback redirect lands back here with
  // ?bitbucket_app=connected on success, mirroring the gitlab-app
  // route's own one-time toast then query-param strip.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('bitbucket_app') === 'connected') {
      toast.add({
        title: 'Bitbucket App authorized.',
        description: 'Repositories are now available to pick from.',
        type: 'success',
      })
      void navigate({ to: '/settings/bitbucket-app', replace: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <GitBranchIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Bitbucket App</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect a Bitbucket Cloud OAuth consumer for repository browsing
            and webhook-driven deploys.
          </p>
        </div>
      </div>
      <BitbucketAppConnectionCard />
      <BitbucketAppReposCard />
    </div>
  )
}
