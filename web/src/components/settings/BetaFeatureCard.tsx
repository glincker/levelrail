import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { BetaFeatureServerOptions } from '@/components/settings/BetaFeatureServerOptions'
import { RestartControlPlaneButton } from '@/components/RestartControlPlaneButton'
import { useIsRoot } from '@/hooks/useIsRoot'
import { useSystemRestartInfo } from '@/queries/systemRestart'
import { HelpLink } from '@/components/HelpLink'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import type { BetaFeature } from '../../types/resourceAdvisor'

export function RestartNeeded() {
  const { t } = useTranslation('betaFeatures')
  const isRoot = useIsRoot()
  const { data: info, refetch, isFetching } = useSystemRestartInfo()
  if (!isRoot) {
    return (
      <p className="text-xs text-muted-foreground">{t('restart.askAdmin')}</p>
    )
  }
  return (
    <div className="space-y-2 rounded-md border border-warning/40 bg-warning/10 p-3">
      <p className="text-sm font-medium">{t('restart.needed')}</p>
      {info?.supported ? (
        <RestartControlPlaneButton variant="default" label={t('restart.now')} />
      ) : (
        <div className="space-y-2">
          <p className="text-xs text-muted-foreground">
            {t('restart.unsupported')}
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={isFetching}
            onClick={() => void refetch()}
          >
            <RefreshCwGlassIcon size="1.2em" aria-hidden="true" />
            {t('restart.recheck')}
          </Button>
          {info?.hint ? (
            <details className="text-xs text-muted-foreground">
              <summary className="cursor-pointer">
                {t('restart.manual')}
              </summary>
              <code className="mt-1 block rounded-md bg-muted px-2 py-1 font-mono">
                {info.hint}
              </code>
            </details>
          ) : null}
        </div>
      )}
    </div>
  )
}

export function BetaFeatureCard({
  feature,
  highlighted,
  pending,
  onToggle,
  detail,
}: {
  feature: BetaFeature
  highlighted: boolean
  pending: boolean
  onToggle: (enabled: boolean) => void
  detail?: string
}) {
  const { t } = useTranslation('betaFeatures')
  const switchId = useId()
  const [optionsOpen, setOptionsOpen] = useState(false)
  const saved = feature.pending_restart ? !feature.enabled : feature.enabled
  const state = feature.enabled
    ? feature.enabled_by === 'env'
      ? t('state.onByEnv')
      : t('state.onBySetting')
    : t('state.off')

  return (
    <Card
      id={`feature-${feature.id}`}
      data-highlighted={highlighted ? 'true' : undefined}
      className={cn(
        'scroll-mt-24',
        highlighted && 'ring-2 ring-primary transition-shadow',
      )}
    >
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          {feature.name}
          <Badge variant="muted">{t(`groups.${feature.maturity}`)}</Badge>
          <Badge variant={feature.enabled ? 'success' : 'outline'}>
            {state}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          {detail ?? feature.summary}
        </p>
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <Switch
              id={switchId}
              checked={saved}
              disabled={feature.locked || pending}
              onCheckedChange={onToggle}
            />
            <Label htmlFor={switchId}>
              {t('toggle.label', { name: feature.name })}
            </Label>
          </div>
          {feature.locked ? (
            <p className="text-xs text-muted-foreground">
              {t('toggle.lockedByEnv')}
            </p>
          ) : null}
        </div>
        {feature.pending_restart ? <RestartNeeded /> : null}
        {feature.locked ? null : (
          <Button
            type="button"
            variant="link"
            size="sm"
            className="h-auto p-0"
            onClick={() => setOptionsOpen(true)}
          >
            {t('options.open')}
          </Button>
        )}
        <BetaFeatureServerOptions
          feature={feature}
          open={optionsOpen}
          onOpenChange={setOptionsOpen}
        />
        <HelpLink path={feature.docs_path} label={t('docs')} variant="inline" />
      </CardContent>
    </Card>
  )
}
