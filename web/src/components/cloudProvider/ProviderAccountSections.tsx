import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { apiRequest } from '../../lib/apiRequest'
import { AWS_CAPABILITIES } from '../../lib/cloudAccounts'
import { providerAccounts } from '../../lib/providerConnectionState'
import { CapabilityResults } from '../cloudAccounts/CapabilityResults'
import { CloudInstancesTab } from '../cloudAccounts/CloudInstancesTab'
import { CloudSpendCard } from '../cloudAccounts/CloudSpendCard'
import { SkeletonList } from '../kit/Skeleton'
import type {
  CloudCapabilityResult,
  CloudVerifyResult,
} from '../../types/cloudAccounts'
import type { SectionProps } from './sections'
import { useProviderAccount } from './useProviderAccount'

const ORACLE_CAPABILITIES = ['compute', 'usage', 'budgets'] as const

// Providers with no probe list fall back to the account verify call.
function probeList(provider: string): readonly string[] {
  if (provider === 'oracle') return ORACLE_CAPABILITIES
  if (provider === 'aws')
    return AWS_CAPABILITIES.filter((c) => c !== 'destructive')
  return []
}

interface AccessCheck {
  probe: CloudCapabilityResult[] | null
  verify: CloudVerifyResult | null
}

async function checkAccess(id: string, provider: string): Promise<AccessCheck> {
  const base = `/api/v1/cloud-accounts/${encodeURIComponent(id)}`
  const caps = probeList(provider)
  if (caps.length > 0) {
    const res = await apiRequest<{ capabilities: CloudCapabilityResult[] }>(
      `${base}/probe`,
      {
        method: 'POST',
        body: { capabilities: caps },
        fallbackMessage: 'probe cloud account failed',
      },
    )
    return { probe: res.capabilities, verify: null }
  }
  const verify = await apiRequest<CloudVerifyResult>(`${base}/verify`, {
    method: 'POST',
    fallbackMessage: 'verify cloud account failed',
  })
  return { probe: null, verify }
}

function AccountSwitcher({
  provider,
  accounts,
}: Readonly<
  Pick<SectionProps, 'provider' | 'resource'> & {
    accounts: ReturnType<typeof providerAccounts>
  }
>) {
  const { t } = useTranslation('cloudProvider')
  const { account, select } = useProviderAccount(provider.id, accounts)
  if (accounts.length < 2) return null
  return (
    <label className="ms-auto flex items-center gap-1.5 text-xs text-muted-foreground">
      {t('accountSection.switch')}
      <select
        value={account?.id ?? ''}
        onChange={(e) => {
          select(e.target.value)
        }}
        className="h-7 rounded-md border border-border bg-background px-1.5 text-xs text-foreground"
      >
        {accounts.map((a) => (
          <option key={a.id} value={a.id}>
            {a.label}
          </option>
        ))}
      </select>
    </label>
  )
}

function Header({
  id,
  title,
  provider,
  resource,
}: Readonly<
  Pick<SectionProps, 'provider' | 'resource'> & { id: string; title: string }
>) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
      <h2 id={id} className="text-sm font-semibold">
        {title}
      </h2>
      <AccountSwitcher
        provider={provider}
        resource={resource}
        accounts={providerAccounts(resource)}
      />
    </div>
  )
}

export function AccountResourcesSection({
  provider,
  resource,
  labels,
}: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const { account } = useProviderAccount(
    provider.id,
    providerAccounts(resource),
  )
  return (
    <section aria-labelledby="cp-resources" className="space-y-2">
      <Header
        id="cp-resources"
        title={t('resources.title', { servers: labels.servers })}
        provider={provider}
        resource={resource}
      />
      {account ? (
        <CloudInstancesTab
          key={account.id}
          accountId={account.id}
          provider={provider.id}
        />
      ) : null}
    </section>
  )
}

export function AccountSpendSection({ provider, resource }: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const { account } = useProviderAccount(
    provider.id,
    providerAccounts(resource),
  )
  return (
    <section aria-labelledby="cp-spend" className="space-y-2">
      <Header
        id="cp-spend"
        title={t('spend.title')}
        provider={provider}
        resource={resource}
      />
      {account ? (
        <CloudSpendCard key={account.id} accountId={account.id} />
      ) : null}
    </section>
  )
}

export function AccountCapabilitiesPanel({ provider, resource }: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const { account } = useProviderAccount(
    provider.id,
    providerAccounts(resource),
  )
  const id = account?.id ?? ''
  const check = useQuery({
    queryKey: ['cloud-accounts', 'provider-access', id],
    enabled: id !== '',
    retry: false,
    staleTime: 60_000,
    queryFn: () => checkAccess(id, provider.id),
  })
  let body
  if (check.isPending) {
    body = <SkeletonList rows={3} />
  } else if (check.isError) {
    body = (
      <div
        role="alert"
        className="flex items-center gap-2 text-sm text-destructive"
      >
        <span>
          {t('accountSection.error', { message: check.error.message })}
        </span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => void check.refetch()}
        >
          {t('accountSection.retry')}
        </Button>
      </div>
    )
  } else if (check.data.probe) {
    body =
      check.data.probe.length > 0 ? (
        <CapabilityResults results={check.data.probe} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('accountSection.noProbe')}
        </p>
      )
  } else if (check.data.verify) {
    const v = check.data.verify
    body = (
      <p
        role="status"
        className={v.ok ? 'text-sm' : 'text-sm text-destructive'}
      >
        {v.ok ? t('accountSection.verifyOk') : t('accountSection.verifyFail')}
        {v.message ? `: ${v.message}` : ''}
      </p>
    )
  }
  return (
    <section
      id="cp-capabilities"
      aria-labelledby="cp-capabilities-title"
      className="space-y-2 rounded-lg border border-border p-3"
    >
      <Header
        id="cp-capabilities-title"
        title={t('capabilities.title')}
        provider={provider}
        resource={resource}
      />
      {body}
      {check.isSuccess ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={check.isFetching}
          onClick={() => void check.refetch()}
        >
          {t('accountSection.check')}
        </Button>
      ) : null}
    </section>
  )
}
