import { useId } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { FlaskIcon } from '@phosphor-icons/react/dist/ssr'
import { PageHeader } from '@/components/shell/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import {
  betaFeaturesQueryOptions,
  useBetaFeatures,
  useSetBetaFeature,
} from '../../queries/resourceAdvisor'

export const Route = createFileRoute('/settings/beta-features')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(betaFeaturesQueryOptions()),
  component: BetaFeaturesPage,
  pendingComponent: () => (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={2} />
    </div>
  ),
})

const ADVISOR = 'resource-advisor'

function BetaFeaturesPage() {
  const { t } = useTranslation('resourceAdvisor')
  const { data: features } = useBetaFeatures()
  const setFeature = useSetBetaFeature()
  const switchId = useId()
  const advisor = features?.find((f) => f.feature === ADVISOR)

  function toggle(enabled: boolean) {
    setFeature.mutate(
      { feature: ADVISOR, enabled },
      {
        onSuccess: (res) =>
          toast.add({
            type: 'success',
            title: res.enabled
              ? t('settings.turnedOn')
              : t('settings.turnedOff'),
          }),
        onError: (err) =>
          toast.add({
            type: 'error',
            title: t('settings.error', { message: err.message }),
          }),
      },
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <FlaskIcon className="size-4" />
        </div>
        <PageHeader
          title={t('settings.title')}
          description={t('settings.description')}
        />
      </div>
      {advisor ? (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              {t('settings.advisorTitle')}
              <Badge variant="outline" title={t('beta.hint')}>
                {t('beta.label')}
              </Badge>
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-muted-foreground">
              {t('settings.advisorDescription')}
            </p>
            <div className="flex items-center gap-3">
              <Switch
                id={switchId}
                checked={advisor.enabled}
                disabled={advisor.locked || setFeature.isPending}
                onCheckedChange={toggle}
              />
              <Label htmlFor={switchId}>{t('settings.toggleLabel')}</Label>
            </div>
            {advisor.locked ? (
              <p className="text-xs text-muted-foreground">
                {t('settings.locked')}
              </p>
            ) : null}
            <p className="text-xs text-muted-foreground">{t('beta.hint')}</p>
          </CardContent>
        </Card>
      ) : null}
    </div>
  )
}
