import { useTranslation } from 'react-i18next'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import type { NodeProviderMethod } from '../../types/nodeProvision'

const PERMISSION_KEYS = {
  hetzner: 'nodeProviders.permissions.hetzner',
  digitalocean: 'nodeProviders.permissions.digitalocean',
  aws: 'nodeProviders.permissions.aws',
  azure: 'nodeProviders.permissions.azure',
  gcp: 'nodeProviders.permissions.gcp',
  custom: 'nodeProviders.permissions.custom',
} as const

function isKnown(p: string): p is keyof typeof PERMISSION_KEYS {
  return p in PERMISSION_KEYS
}

// Least privilege shown inside the dialog so an operator can scope the
// credential before pasting it, not after.
export function NodeProviderPermissions({
  provider,
  method,
}: {
  provider: string
  method: NodeProviderMethod
}) {
  const { t } = useTranslation('settings')
  if (!isKnown(provider)) return null
  return (
    <section
      aria-label={t('nodeProviders.permissionsTitle')}
      className="space-y-1 rounded-lg border border-border bg-muted/30 p-3 text-xs"
    >
      <h3 className="flex items-center gap-1.5 text-sm font-medium">
        <ShieldCheckIcon aria-hidden="true" className="size-4" />
        {t('nodeProviders.permissionsTitle')}
      </h3>
      <p className="text-muted-foreground">{t(PERMISSION_KEYS[provider])}</p>
      {provider === 'aws' && method === 'assume_role' ? (
        <p className="text-muted-foreground">
          {t('nodeProviders.permissions.awsRole')}
        </p>
      ) : null}
    </section>
  )
}
