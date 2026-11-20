import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { BuildTrustTier } from '../../queries/builderTrust'

const TIER_VARIANT = {
  server: 'success',
  managed: 'info',
  personal: 'warning',
} as const

export function BuildTrustBadge({ tier }: Readonly<{ tier: BuildTrustTier }>) {
  const { t } = useTranslation('nodes')
  return (
    <Badge variant={TIER_VARIANT[tier]}>{t(`builderTrust.tier.${tier}`)}</Badge>
  )
}
