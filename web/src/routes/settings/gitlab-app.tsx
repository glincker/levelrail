import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { GitlabLogoIcon } from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { PageHeader } from '@/components/shell/PageHeader'
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
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <GitlabLogoIcon className="size-4" />
        </div>
        <PageHeader
          title="GitLab App"
          description="Connect a GitLab OAuth Application, gitlab.com or self-hosted, for project browsing and webhook-driven deploys."
        />
      </div>
      <GitLabAppConnectionCard />
      <GitLabAppProjectsCard />
    </div>
  )
}
