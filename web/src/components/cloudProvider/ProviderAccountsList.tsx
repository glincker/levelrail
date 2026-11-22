import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { StatusPill } from '../kit/StatusPill'
import { RelativeTime } from '../kit/RelativeTime'
import { providerAccounts } from '../../lib/providerConnectionState'
import type { SectionProps } from './sections'

// Cloud accounts are the primary connection; AWS links through its own flow,
// every other provider reuses the shared connect dialog.
export function ProviderAccountsList({
  provider,
  resource,
  connection,
}: Pick<SectionProps, 'provider' | 'resource' | 'connection'>) {
  const { t } = useTranslation('cloudProvider')
  const accounts = providerAccounts(resource)
  return (
    <div className="space-y-2">
      <ul className="space-y-1.5" aria-label={t('linkedAccounts.title')}>
        {accounts.map((a) => {
          const ok = a.status === 'active'
          return (
            <li
              key={a.id}
              className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs"
            >
              <span className="font-medium">{a.label}</span>
              <StatusPill
                tone={a.needs_relogin ? 'warning' : ok ? 'success' : 'danger'}
                label={
                  a.needs_relogin
                    ? t('linkedAccounts.relogin')
                    : ok
                      ? t('linkedAccounts.active')
                      : t('linkedAccounts.attention')
                }
                size="sm"
              />
              {a.region ? (
                <span className="text-muted-foreground">{a.region}</span>
              ) : null}
              {a.last_verified_at ? (
                <span className="text-muted-foreground">
                  {t('linkedAccounts.verified')}{' '}
                  <RelativeTime at={a.last_verified_at} />
                </span>
              ) : null}
              <Link
                to="/integrations/cloud-accounts/$id"
                params={{ id: a.id }}
                className="underline-offset-2 hover:underline"
              >
                {t('linkedAccounts.manageShort')}
              </Link>
            </li>
          )
        })}
      </ul>
      {provider.id === 'aws' ? (
        <Button
          variant="outline"
          size="sm"
          render={<Link to="/integrations/cloud-accounts/aws-link" />}
        >
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('linkedAccounts.addAnother')}
        </Button>
      ) : (
        <Button variant="outline" size="sm" onClick={connection.openConnect}>
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('linkedAccounts.addAnother')}
        </Button>
      )}
    </div>
  )
}
