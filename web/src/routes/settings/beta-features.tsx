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

interface BetaCard {
  feature: 'resource-advisor' | 'ai-chat' | 'cli-console' | 'mobile-api'
  title: string
  description: string
  toggleLabel: string
  turnedOn: string
  turnedOff: string
}

function BetaFeaturesPage() {
  const { t } = useTranslation('resourceAdvisor')
  const { data: features } = useBetaFeatures()
  const setFeature = useSetBetaFeature()
  const idPrefix = useId()
  const cards: BetaCard[] = [
    {
      feature: 'ai-chat',
      title: t('settings.aiChatTitle'),
      description: t('settings.aiChatDescription'),
      toggleLabel: t('settings.aiChatToggleLabel'),
      turnedOn: t('settings.aiChatTurnedOn'),
      turnedOff: t('settings.aiChatTurnedOff'),
    },
    {
      feature: 'cli-console',
      title: t('settings.cliConsoleTitle'),
      description: t('settings.cliConsoleDescription'),
      toggleLabel: t('settings.cliConsoleToggleLabel'),
      turnedOn: t('settings.cliConsoleTurnedOn'),
      turnedOff: t('settings.cliConsoleTurnedOff'),
    },
    {
      feature: 'mobile-api',
      title: t('settings.mobileApiTitle'),
      description: t('settings.mobileApiDescription'),
      toggleLabel: t('settings.mobileApiToggleLabel'),
      turnedOn: t('settings.mobileApiTurnedOn'),
      turnedOff: t('settings.mobileApiTurnedOff'),
    },
    {
      feature: 'resource-advisor',
      title: t('settings.advisorTitle'),
      description: t('settings.advisorDescription'),
      toggleLabel: t('settings.toggleLabel'),
      turnedOn: t('settings.turnedOn'),
      turnedOff: t('settings.turnedOff'),
    },
  ]

  function toggle(card: BetaCard, enabled: boolean) {
    setFeature.mutate(
      { feature: card.feature, enabled },
      {
        onSuccess: (res) =>
          toast.add({
            type: 'success',
            title: res.enabled ? card.turnedOn : card.turnedOff,
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
      {cards.map((card) => {
        const state = features?.find((f) => f.feature === card.feature)
        if (!state) return null
        const id = `${idPrefix}-${card.feature}`
        return (
          <Card key={card.feature}>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                {card.title}
                <Badge variant="outline" title={t('beta.hint')}>
                  {t('beta.label')}
                </Badge>
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <p className="text-sm text-muted-foreground">
                {card.description}
              </p>
              <div className="flex items-center gap-3">
                <Switch
                  id={id}
                  checked={state.enabled}
                  disabled={state.locked || setFeature.isPending}
                  onCheckedChange={(v) => toggle(card, v)}
                />
                <Label htmlFor={id}>{card.toggleLabel}</Label>
              </div>
              {state.locked ? (
                <p className="text-xs text-muted-foreground">
                  {t('settings.locked')}
                </p>
              ) : null}
              <p className="text-xs text-muted-foreground">{t('beta.hint')}</p>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
