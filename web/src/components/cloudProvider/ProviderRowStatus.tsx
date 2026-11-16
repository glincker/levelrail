import { useTranslation } from 'react-i18next'
import { capabilitySummary } from '../../lib/providerCapabilities'
import { useProviderCapabilities } from '../../queries/providerCapabilities'

// One short line under a connected provider: only real API data, never a guess.
export function ProviderRowStatus({
  provider,
}: Readonly<{ provider: string }>) {
  const { t } = useTranslation('cloudProvider')
  const caps = useProviderCapabilities(provider, provider !== 'custom')
  if (caps.isPending) return null
  const text =
    caps.data && caps.data.capabilities.length > 0
      ? t('capabilities.rowSummary', capabilitySummary(caps.data))
      : t('capabilities.notChecked')
  return <p className="truncate text-xs text-muted-foreground">{text}</p>
}
