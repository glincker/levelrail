import { useTranslation } from 'react-i18next'
import { NativeSelect } from '@/components/ui/native-select'
import { toast } from '@/components/ui/toast'
import {
  BUILD_TRUST_TIERS,
  isBuildTrustTier,
  useSetNodeBuildTrust,
} from '../../queries/builderTrust'

// Shared by the node page card and the builders table so both save the same way.
export function BuildTierSelect({
  nodeId,
  value,
  id,
  label,
  disabled,
  className,
}: Readonly<{
  nodeId: string
  value: string
  id: string
  label?: string
  disabled?: boolean
  className?: string
}>) {
  const { t } = useTranslation('nodes')
  const setTrust = useSetNodeBuildTrust()
  const known = isBuildTrustTier(value)
  return (
    <NativeSelect
      id={id}
      aria-label={label}
      className={className}
      value={known ? value : ''}
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
      {known ? null : <option value="">{t('builderTrust.unknown')}</option>}
      {BUILD_TRUST_TIERS.map((tier) => (
        <option key={tier} value={tier}>
          {t(`builderTrust.tier.${tier}`)}
        </option>
      ))}
    </NativeSelect>
  )
}
