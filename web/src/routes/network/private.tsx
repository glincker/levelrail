import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { PageHeader } from '@/components/shell/PageHeader'
import { PrivateNetworkMap } from '../../components/network/private/PrivateNetworkMap'
import { PrivateNetworksPanel } from '../../components/network/private/PrivateNetworksPanel'
import { PrivateRulesPanel } from '../../components/network/private/PrivateRulesPanel'
import { useIsRoot } from '../../hooks/useIsRoot'
import { routeErrorMessage } from '../../lib/apiError'
import {
  useApplyPrivateNetworks,
  usePrivateNetworkMap,
  usePrivateNetworks,
} from '../../queries/privateNetworks'

export const Route = createFileRoute('/network/private')({
  component: PrivateNetworksPage,
})

function PrivateNetworksPage() {
  const { t } = useTranslation('privateNetworks')
  const isRoot = useIsRoot()
  const list = usePrivateNetworks()
  const map = usePrivateNetworkMap()
  const apply = useApplyPrivateNetworks()

  const failure = list.error ?? map.error
  if (failure) {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          {t('errors.load')} {routeErrorMessage(failure)}
        </AlertDescription>
      </Alert>
    )
  }
  if (!list.data || !map.data) {
    return (
      <div className="space-y-4" aria-hidden="true">
        <Skeleton className="h-6 w-48" />
        <Skeleton className="h-64 w-full" />
      </div>
    )
  }

  const appNames = map.data.apps.map((a) => a.name)

  return (
    <div className="space-y-4">
      <PageHeader
        title={t('page.title')}
        description={t('page.description')}
        actions={
          isRoot ? (
            <Button
              variant="outline"
              disabled={apply.isPending}
              onClick={() =>
                apply.mutate(undefined, {
                  onSuccess: (r) =>
                    toast.add({
                      title: r.enforced
                        ? t('page.applied')
                        : t('page.notApplied'),
                      type: r.enforced ? 'success' : 'error',
                    }),
                  onError: (err) =>
                    toast.add({ title: err.message, type: 'error' }),
                })
              }
            >
              {t('page.apply')}
            </Button>
          ) : null
        }
      />
      <PrivateNetworkMap data={map.data} />
      <PrivateNetworksPanel networks={list.data.networks} apps={appNames} />
      <PrivateRulesPanel rules={list.data.rules} apps={appNames} />
    </div>
  )
}
