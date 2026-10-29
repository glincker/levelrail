import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDeployFromCache } from '../queries/deployFromCache'

/** DeployFromCacheButton redeploys the last known good commit while the git host is down. */
export function DeployFromCacheButton({ appName }: { appName: string }) {
  const { t } = useTranslation('gitBuild')
  const deploy = useDeployFromCache(appName)

  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="relative z-10 mt-2"
      disabled={deploy.isPending}
      onClick={() =>
        deploy.mutate(undefined, {
          onSuccess: () =>
            toast.add({ title: t('fromCache.started'), type: 'success' }),
          onError: (error) =>
            toast.add({
              title: t('fromCache.failed'),
              description: error.message,
              type: 'error',
            }),
        })
      }
    >
      <DatabaseGlassIcon className="size-5" data-icon="inline-start" />
      {deploy.isPending ? t('fromCache.pending') : t('fromCache.action')}
    </Button>
  )
}
