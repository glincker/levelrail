import { useTranslation } from 'react-i18next'
import { ArrowsClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useCheckImageUpdate,
  useImageAutoUpdate,
  useSetImageAutoUpdate,
} from '../queries/imageAutoUpdate'

/** ImageAutoUpdateCard opts an app into redeploying when its image tag moves, and checks the registry on demand. */
export function ImageAutoUpdateCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const setting = useImageAutoUpdate(appName)
  const setEnabled = useSetImageAutoUpdate(appName)
  const check = useCheckImageUpdate(appName)

  function toggle(next: boolean) {
    setEnabled.mutate(next, {
      onSuccess: () =>
        toast.add({
          title: next
            ? t('imageAutoUpdate.toast.enabled')
            : t('imageAutoUpdate.toast.disabled'),
          type: 'success',
        }),
      onError: (error) =>
        toast.add({
          title: t('imageAutoUpdate.toast.errorTitle'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  function checkNow() {
    check.mutate(undefined, {
      onError: (error) =>
        toast.add({
          title: t('imageAutoUpdate.toast.checkFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  const { last_result: lastResult, last_checked_at: lastChecked } = setting.data

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ArrowsClockwiseIcon className="size-4 text-muted-foreground" />
          {t('imageAutoUpdate.title')}
          <InfoTip label={t('imageAutoUpdate.infoTipLabel')}>
            {t('imageAutoUpdate.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm font-medium text-foreground">
            {t('imageAutoUpdate.enabledLabel')}
          </p>
          <Switch
            checked={setting.data.enabled}
            onCheckedChange={toggle}
            disabled={setEnabled.isPending}
            aria-label={t('imageAutoUpdate.switchAriaLabel')}
          />
        </div>
        <div className="flex items-center justify-between gap-4">
          <p className="text-xs text-muted-foreground" role="status">
            {lastResult
              ? t('imageAutoUpdate.lastCheck', {
                  result: lastResult,
                  when: lastChecked
                    ? new Date(lastChecked).toLocaleString()
                    : '',
                })
              : t('imageAutoUpdate.neverChecked')}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={checkNow}
            disabled={check.isPending}
          >
            {check.isPending
              ? t('imageAutoUpdate.checking')
              : t('imageAutoUpdate.checkNow')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
