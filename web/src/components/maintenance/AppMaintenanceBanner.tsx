import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useAppMaintenance,
  useClearAppMaintenance,
} from '../../queries/appMaintenance'

// One banner for the whole app, so the app pages never show one per domain.
export function AppMaintenanceBanner({ appName }: { appName: string }) {
  const { t } = useTranslation('maintenance')
  const { data } = useAppMaintenance(appName)
  const clear = useClearAppMaintenance(appName)
  if (!data?.enabled) return null
  return (
    <Banner
      variant="warning"
      title={t('appBanner.title', {
        count: data.active_domains,
        total: data.total_domains,
      })}
      action={
        <Button
          size="sm"
          variant="outline"
          disabled={clear.isPending}
          onClick={() => {
            clear.mutate(undefined, {
              onSuccess: () => {
                toast.add({ title: t('appBanner.ended'), type: 'success' })
              },
              onError: (err) => {
                toast.add({ title: err.message, type: 'error' })
              },
            })
          }}
        >
          {clear.isPending ? t('appBanner.ending') : t('appBanner.end')}
        </Button>
      }
    >
      <p>{t('appBanner.body')}</p>
    </Banner>
  )
}
