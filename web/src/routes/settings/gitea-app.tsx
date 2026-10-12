import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { toast } from '@/components/ui/toast'
import { GitProviderPage } from '@/components/settings/GitProviderPage'
import { GiteaAppConnectionCard } from '../../components/GiteaAppConnectionCard'
import { GiteaAppReposCard } from '../../components/GiteaAppReposCard'
import { giteaAppStatusQueryOptions } from '../../queries/giteaApp'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'

// Account-level, mirroring routes/settings/bitbucket-app.tsx's own
// structure and placement.
export const Route = createFileRoute('/settings/gitea-app')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(giteaAppStatusQueryOptions()),
  component: GiteaAppSettingsPage,
  pendingComponent: GiteaAppSettingsSkeleton,
})

function GiteaAppSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={1} rowVariant="status" />
      <SettingsCardSkeleton headerIcon rows={3} rowVariant="list" />
    </div>
  )
}

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
    <GitProviderPage provider="gitea">
      <GiteaAppConnectionCard />
      <GiteaAppReposCard />
    </GitProviderPage>
  )
}
