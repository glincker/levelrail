import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { HelpLink } from '@/components/HelpLink'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import type { BetaFeature } from '../../types/resourceAdvisor'

const ENV_VAR = 'APP_EXPERIMENTAL'

function EnvLine({ feature }: { feature: BetaFeature }) {
  const { t } = useTranslation('betaFeatures')
  const [copied, setCopied] = useState(false)
  const line = `${ENV_VAR}=${feature.id}`

  function copy() {
    void navigator.clipboard.writeText(line).then(
      () => {
        setCopied(true)
        window.setTimeout(() => setCopied(false), 1500)
      },
      () => undefined,
    )
  }

  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t('env.heading')}</p>
      <div className="flex items-center gap-2">
        <code
          aria-label={t('env.line')}
          className="rounded-md bg-muted px-2 py-1 font-mono text-xs"
        >
          {line}
        </code>
        <Button type="button" variant="outline" size="sm" onClick={copy}>
          {copied ? (
            <CheckIcon aria-hidden="true" />
          ) : (
            <CopyIcon aria-hidden="true" />
          )}
          {copied ? t('env.copied') : t('env.copy')}
        </Button>
      </div>
      <ul className="space-y-1 text-xs text-muted-foreground">
        <li>
          <span className="font-medium">{t('env.systemd')}</span>
          {': '}
          {t('env.systemdHow', { line })}
        </li>
        <li>
          <span className="font-medium">{t('env.docker')}</span>
          {': '}
          {t('env.dockerHow', { line })}
        </li>
      </ul>
      {feature.requires_restart ? (
        <p className="text-xs text-muted-foreground">{t('env.restart')}</p>
      ) : null}
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
        {feature.toggleable ? (
          <div className="space-y-2">
            <div className="flex items-center gap-3">
              <Switch
                id={switchId}
                checked={feature.enabled}
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
        ) : feature.enabled ? null : (
          <EnvLine feature={feature} />
        )}
        <HelpLink path={feature.docs_path} label={t('docs')} variant="inline" />
      </CardContent>
    </Card>
  )
}
