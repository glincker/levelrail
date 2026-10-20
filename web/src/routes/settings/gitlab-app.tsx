import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { toast } from '@/components/ui/toast'
import { GitProviderPage } from '@/components/settings/GitProviderPage'
import { GitLabAppConnectionCard } from '../../components/GitLabAppConnectionCard'
import { GitLabAppProjectsCard } from '../../components/GitLabAppProjectsCard'
import { gitlabAppStatusQueryOptions } from '../../queries/gitlabApp'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'

// Account-level, mirroring routes/settings/github-app.tsx's own
// structure and placement.
export const Route = createFileRoute('/settings/gitlab-app')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(gitlabAppStatusQueryOptions()),
  component: GitLabAppSettingsPage,
  pendingComponent: GitLabAppSettingsSkeleton,
})

function GitLabAppSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={1} rowVariant="status" />
      <SettingsCardSkeleton headerIcon rows={3} rowVariant="list" />
    </div>
  )
}

function GitLabAppSettingsPage() {
  useSuspenseQuery(gitlabAppStatusQueryOptions())
  const navigate = useNavigate()

  // GitLab's OAuth callback redirect lands back here with
  // ?gitlab_app=connected on success, mirroring the github-app route's
  // own one-time toast then query-param strip.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('gitlab_app') === 'connected') {
      toast.add({
        title: 'GitLab App authorized.',
        description: 'Projects are now available to pick from.',
        type: 'success',
      })
      void navigate({ to: '/settings/gitlab-app', replace: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <GitProviderPage provider="gitlab">
      <GitLabAppConnectionCard />
      <GitLabAppProjectsCard />
    </GitProviderPage>
  )
}
