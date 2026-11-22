import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { StatusPill } from '../kit/StatusPill'
import type { NodeProviderResource } from '../../types/nodeProvision'

// Cloud accounts are the primary credential; the legacy single credential
// stays visible but is labelled so there is one place to manage.
export function NodeProviderAccounts({
  provider,
}: {
  provider: NodeProviderResource
}) {
  const { t } = useTranslation('cloudProvider')
  const accounts = provider.accounts ?? []
  if (accounts.length === 0 && provider.provider === 'custom') return null
  const hasOwnAccount = accounts.some((a) => !a.managed)

  return (
    <div className="relative z-10 w-full space-y-1 pb-1 pl-1">
      {accounts.length > 0 ? (
        <ul className="space-y-1" aria-label={t('linkedAccounts.title')}>
          {accounts.map((a) => {
            const tone = a.needs_relogin
              ? 'warning'
              : a.status === 'active'
                ? 'success'
                : 'danger'
            const label = a.needs_relogin
              ? t('linkedAccounts.relogin')
              : a.status === 'active'
                ? t('linkedAccounts.active')
                : t('linkedAccounts.attention')
            return (
              <li
                key={a.id}
                className="flex flex-wrap items-center gap-2 text-xs"
              >
                <span className="font-medium">{a.label}</span>
                {a.region ? (
                  <span className="text-muted-foreground">{a.region}</span>
                ) : null}
                {a.managed ? (
                  <span className="text-muted-foreground">
                    {t('linkedAccounts.managed')}
                  </span>
                ) : null}
                <StatusPill tone={tone} label={label} size="sm" />
                <Link
                  to="/integrations/cloud-accounts/$id"
                  params={{ id: a.id }}
                  className="underline-offset-2 hover:underline"
                >
                  {t('linkedAccounts.manage')}
                </Link>
              </li>
            )
          })}
        </ul>
      ) : null}
      <p className="text-xs">
        <Link
          to="/integrations/cloud/$provider"
          params={{ provider: provider.provider }}
          className="underline-offset-2 hover:underline"
        >
          {t('linkedAccounts.add')}
        </Link>
      </p>
      {hasOwnAccount && provider.has_token ? (
        <p className="text-xs text-muted-foreground">
          {t('linkedAccounts.legacyHint')}
        </p>
      ) : null}
    </div>
  )
}
