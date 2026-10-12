import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { toast } from '@/components/ui/toast'
import { BitbucketAppConnectionCard } from '../../components/BitbucketAppConnectionCard'
import { BitbucketAppReposCard } from '../../components/BitbucketAppReposCard'
import { bitbucketAppStatusQueryOptions } from '../../queries/bitbucketApp'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'
import { GitProviderPage } from '@/components/settings/GitProviderPage'

// Account-level, mirroring routes/settings/gitlab-app.tsx's own
// structure and placement.
export const Route = createFileRoute('/settings/bitbucket-app')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(bitbucketAppStatusQueryOptions()),
  component: BitbucketAppSettingsPage,
  pendingComponent: BitbucketAppSettingsSkeleton,
})

function BitbucketAppSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={1} rowVariant="status" />
      <SettingsCardSkeleton headerIcon rows={3} rowVariant="list" />
    </div>
  )
}

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
    <GitProviderPage provider="bitbucket">
      <BitbucketAppConnectionCard />
      <BitbucketAppReposCard />
    </GitProviderPage>
  )
}
