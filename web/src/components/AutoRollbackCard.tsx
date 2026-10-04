import { useTranslation } from 'react-i18next'
import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { InfoTip } from '@/components/kit'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useAutoRollback, useSetAutoRollback } from '../queries/autoRollback'

// AutoRollbackCard is the opt-in toggle for
// internal/alerting.MaybeAutoRollback (GET/PUT
// /api/v1/apps/{name}/auto-rollback): off by default, the same
// risky-by-default-feature-is-opt-in shape PreviewEnvironmentsCard's own
// "Enabled" toggle already establishes. Rendered above DeployAttemptsList
// on the Deploys route, since this setting only makes sense next to real
// deploy history.
export function AutoRollbackCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const setting = useAutoRollback(appName)
  const setAutoRollback = useSetAutoRollback(appName)

  function toggle(next: boolean) {
    setAutoRollback.mutate(next, {
      onSuccess: () => {
        toast.add({
          title: next
            ? t('autoRollback.toast.enabled')
            : t('autoRollback.toast.disabled'),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: t('autoRollback.toast.errorTitle'),
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
          <ArrowCounterClockwiseIcon className="size-4 text-muted-foreground" />
          {t('autoRollback.title')}
          <InfoTip
            label={t('autoRollback.infoTipLabel')}
            helpPath="/observability#alert-rules"
            helpLabel={t('autoRollback.helpLabel')}
          >
            {t('autoRollback.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <p className="text-sm font-medium text-foreground">
            {t('autoRollback.enabledLabel')}
          </p>
          <Switch
            checked={setting.data.enabled}
            onCheckedChange={toggle}
            disabled={setAutoRollback.isPending}
            aria-label={t('autoRollback.switchAriaLabel')}
          />
        </div>
      </CardContent>
    </Card>
  )
}
