import { useTranslation } from 'react-i18next'
import { SkipForwardGlassIcon } from '@thesvg/ui-react'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { InfoTip } from './kit'
import {
  useCancelSuperseded,
  useSetCancelSuperseded,
} from '../queries/deployControl'

// Per-app opt-in (GET/PUT /api/v1/apps/{name}/cancel-superseded): off by
// default so no queued deploy is ever dropped unless the operator asks.
export function CancelSupersededCard({ appName }: { appName: string }) {
  const { t } = useTranslation('common')
  const setting = useCancelSuperseded(appName)
  const setSetting = useSetCancelSuperseded(appName)

  // A switch drawn off before data arrives would show a wrong state.
  if (setting.isPending) return <CardSkeleton titleWidth="w-72" rows={1} />

  function toggle(next: boolean) {
    setSetting.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: next ? t('cancelSuperseded.on') : t('cancelSuperseded.off'),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: t('cancelSuperseded.saveFailed'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SkipForwardGlassIcon className="size-4.5 text-muted-foreground" />
          {t('cancelSuperseded.title')}
          <InfoTip label={t('cancelSuperseded.infoLabel')}>
            {t('cancelSuperseded.infoBody')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">
              {t('cancelSuperseded.enabled')}
            </p>
            <p className="text-sm text-muted-foreground">
              {t('cancelSuperseded.enabledHint')}
            </p>
          </div>
          <Switch
            checked={setting.data?.enabled ?? false}
            onCheckedChange={toggle}
            disabled={setting.isPending || setSetting.isPending}
            aria-label={t('cancelSuperseded.title')}
          />
        </div>
      </CardContent>
    </Card>
  )
}
