import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CloudGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { StatusBadge } from '@/components/ui/status-badge'
import { useCloudAccountActions } from '../../hooks/useCloudAccountActions'
import { accountStatusState } from '../../lib/cloudAccounts'
import { formatRelativeTime } from '../../lib/relativeTime'
import { useCloudAccounts } from '../../queries/cloudAccounts'
import { OracleCredentialForm } from '../nodeProviders/OracleCredentialForm'
import { CloudProviderMark } from './CloudProviderMark'
import { DigitalOceanConnect } from './DigitalOceanConnect'
import { DoctorPanel } from './DoctorPanel'

const HEADING =
  'font-(family-name:--glinr-font-display) text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground'

// Oracle opens its guided form here and creates a cloud account, so no subnet is asked for.
export function CloudAccountsSection() {
  const { t } = useTranslation('cloudAccounts')
  const { data, isPending, isError, refetch } = useCloudAccounts()
  const actions = useCloudAccountActions()
  const [oracleOpen, setOracleOpen] = useState(false)
  const accounts = data ?? []

  return (
    <section aria-labelledby="cloud-accounts-title" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="cloud-accounts-title" className={HEADING}>
          {t('section.title')}
        </h2>
        {actions.create ? (
          <div className="flex flex-wrap items-center gap-2">
            <DigitalOceanConnect />
            <Button
              variant="outline"
              size="sm"
              onClick={() => setOracleOpen(true)}
            >
              <PlusGlassIcon size="1.4em" aria-hidden="true" />
              {t('section.addOracle')}
            </Button>
            <Button
              size="sm"
              render={<Link to="/integrations/cloud-accounts/aws-link" />}
            >
              <PlusGlassIcon size="1.4em" aria-hidden="true" />
              {t('section.linkAws')}
            </Button>
          </div>
        ) : null}
      </div>
      {isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t('section.loading')}
        </p>
      ) : isError ? (
        <EmptyState
          kind="error"
          compact
          icon={<CloudGlassIcon className="size-6" />}
          title={t('section.loadFailed')}
          action={
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void refetch()
              }}
            >
              {t('common.retry')}
            </Button>
          }
        />
      ) : accounts.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('section.empty')}</p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {accounts.map((a) => (
            <li
              key={a.id}
              className="flex flex-wrap items-center gap-x-4 gap-y-1 px-3 py-2"
            >
              <CloudProviderMark provider={a.provider} />
              <Link
                to="/integrations/cloud-accounts/$id"
                params={{ id: a.id }}
                className="min-w-0 flex-1 truncate text-sm font-medium text-foreground hover:underline focus-visible:underline"
              >
                {a.label}
              </Link>
              <span className="text-xs text-muted-foreground">
                {t(`providers.${a.provider}`, { defaultValue: a.provider })}
              </span>
              {a.config.region ? (
                <span className="font-mono text-xs text-muted-foreground">
                  {a.config.region}
                </span>
              ) : null}
              <span className="text-xs text-muted-foreground">
                {a.last_verified_at
                  ? t('section.verified', {
                      when: formatRelativeTime(a.last_verified_at),
                    })
                  : t('section.neverVerified')}
              </span>
              <StatusBadge
                state={accountStatusState(a.status)}
                label={t(`status.${a.status}`, { defaultValue: a.status })}
              />
              {a.last_verified_at ? null : (
                <div className="basis-full">
                  <DoctorPanel id={a.id} compact />
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      <Dialog open={oracleOpen} onOpenChange={setOracleOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('section.addOracleTitle')}</DialogTitle>
            <DialogDescription>
              {t('section.addOracleDescription')}
            </DialogDescription>
          </DialogHeader>
          <OracleCredentialForm
            provider={{ provider: 'oracle', has_token: false }}
            label={t('providers.oracle')}
            requireSubnet={false}
            cloudAccount
            onSaved={() => setOracleOpen(false)}
          />
        </DialogContent>
      </Dialog>
    </section>
  )
}
