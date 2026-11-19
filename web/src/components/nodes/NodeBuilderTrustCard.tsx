import { useTranslation } from 'react-i18next'
import { ShieldGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import { InfoTip } from '../kit/InfoTip'
import { useBuildRunners } from '../../queries/buildRunners'
import {
  BUILD_TRUST_TIERS,
  isBuildTrustTier,
  useSetNodeBuildTrust,
  type BuildTrustTier,
} from '../../queries/builderTrust'

const TIER_VARIANT = {
  server: 'success',
  managed: 'info',
  personal: 'warning',
} as const

function BuildTrustBadge({ tier }: Readonly<{ tier: BuildTrustTier }>) {
  const { t } = useTranslation('nodes')
  return (
    <Badge variant={TIER_VARIANT[tier]}>{t(`builderTrust.tier.${tier}`)}</Badge>
  )
}

// Only rendered for build-capable nodes. The current tier comes from the
// runners list, which already reports it per node.
export default function NodeBuilderTrustCard({
  nodeId,
  disabled,
}: Readonly<{ nodeId: string; disabled?: boolean }>) {
  const { t } = useTranslation('nodes')
  const { data } = useBuildRunners()
  const setTrust = useSetNodeBuildTrust()
  const runner = (data?.runners ?? []).find((r) => r.id === nodeId)
  const raw = runner?.trust ?? ''
  const tier = isBuildTrustTier(raw) ? raw : null
  // The runners list only holds build-capable nodes, so others render nothing.
  const selectId = `node-build-trust-${nodeId}`

  if (runner === undefined) return null

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3">
        <CardTitle className="flex items-center gap-2">
          <ShieldGlassIcon aria-hidden="true" className="size-4" />
          {t('builderTrust.title')}
          <InfoTip label={t('builderTrust.tipLabel')}>
            {t('builderTrust.tip')}
          </InfoTip>
        </CardTitle>
        {tier ? <BuildTrustBadge tier={tier} /> : null}
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <Label htmlFor={selectId}>{t('builderTrust.tierLabel')}</Label>
          <NativeSelect
            id={selectId}
            className="w-44"
            value={tier ?? ''}
            disabled={disabled || setTrust.isPending}
            onChange={(e) => {
              const next = e.target.value
              if (!isBuildTrustTier(next)) return
              setTrust.mutate(
                { id: nodeId, tier: next },
                {
                  onSuccess: () =>
                    toast.add({
                      title: t('builderTrust.saved', {
                        tier: t(`builderTrust.tier.${next}`),
                      }),
                      type: 'success',
                    }),
                },
              )
            }}
          >
            {tier ? null : (
              <option value="">{t('builderTrust.unknown')}</option>
            )}
            {BUILD_TRUST_TIERS.map((value) => (
              <option key={value} value={value}>
                {t(`builderTrust.tier.${value}`)}
              </option>
            ))}
          </NativeSelect>
          {tier ? (
            <span className="text-xs text-muted-foreground">
              {t(`builderTrust.hint.${tier}`)}
            </span>
          ) : null}
        </div>
        {setTrust.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{setTrust.error.message}</AlertDescription>
          </Alert>
        ) : null}
      </CardContent>
    </Card>
  )
}
