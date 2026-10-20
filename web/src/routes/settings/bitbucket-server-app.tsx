import { useEffect } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { GitProviderPage } from '@/components/settings/GitProviderPage'
import { GitConnectionsCard } from '@/components/settings/GitConnectionsCard'
import { gitConnectionsQueryOptions } from '../../queries/gitConnections'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'

// Account-level: every Bitbucket Server account connected to this control plane.
export const Route = createFileRoute('/settings/bitbucket-server-app')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(gitConnectionsQueryOptions('bitbucket_server')),
  component: BitbucketServerSettingsPage,
  pendingComponent: BitbucketServerSettingsSkeleton,
})

function BitbucketServerSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={3} rowVariant="list" />
    </div>
  )
}

function BitbucketServerSettingsPage() {
  const navigate = useNavigate()
  const { t } = useTranslation('settings')

  // The OAuth callback lands back here with ?git_connection=connected.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('git_connection') === 'connected') {
      toast.add({
        title: t('gitConnections.toast.authorized'),
        description: t('gitConnections.toast.authorizedBody'),
        type: 'success',
      })
      void navigate({ to: '/settings/bitbucket-server-app', replace: true })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <GitProviderPage provider="bitbucket_server">
      <GitConnectionsCard provider="bitbucket_server" />
    </GitProviderPage>
  )
}
