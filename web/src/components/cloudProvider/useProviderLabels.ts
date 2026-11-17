import { useTranslation } from 'react-i18next'
import type { CloudProviderId } from '../../lib/cloudProviderBrand'

// Every provider word on the page comes from providers.<id>.* so the shared
// components never hardcode one provider's vocabulary.
export function useProviderLabels(id: CloudProviderId) {
  const { t } = useTranslation('cloudProvider')
  return {
    name: t(`providers.${id}.name`),
    tagline: t(`providers.${id}.tagline`),
    server: t(`providers.${id}.terms.server`),
    servers: t(`providers.${id}.terms.servers`),
    location: t(`providers.${id}.terms.location`),
    locations: t(`providers.${id}.terms.locations`),
    size: t(`providers.${id}.terms.size`),
    vocab: t(`providers.${id}.terms.vocab`),
    createLabel: t(`providers.${id}.createLabel`),
    tokenHelp: t(`providers.${id}.tokenHelp`),
  }
}

export type ProviderLabels = ReturnType<typeof useProviderLabels>
