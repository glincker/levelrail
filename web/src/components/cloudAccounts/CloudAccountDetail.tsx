import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CloudGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { StatusBadge } from '@/components/ui/status-badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useIsRoot } from '../../hooks/useIsRoot'
import { cloudAccountActions, deleteConfirmName } from '../../lib/cloudAccess'
import { AWS_CAPABILITIES, accountStatusState } from '../../lib/cloudAccounts'
import { formatAbsoluteTime } from '../../lib/relativeTime'
import {
  useCloudAccount,
  useCloudAccountRegions,
  useDeleteCloudAccount,
  useProbeCloudAccount,
  useVerifyCloudAccount,
} from '../../queries/cloudAccounts'
import type { CloudVerifyResult } from '../../types/cloudAccounts'
import { CapabilityResults } from './CapabilityResults'
import { CloudAccountAccess } from './CloudAccountAccess'
import { CloudProviderMark } from './CloudProviderMark'

function messageOf(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

export function CloudAccountDetail({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const navigate = useNavigate()
  const actions = cloudAccountActions(useIsRoot())
  const account = useCloudAccount(id)
  const regions = useCloudAccountRegions(id, account.isSuccess)
  const verify = useVerifyCloudAccount()
  const probe = useProbeCloudAccount()
  const remove = useDeleteCloudAccount()
  const [verifyResult, setVerifyResult] = useState<CloudVerifyResult | null>(
    null,
  )
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)

  if (account.isPending) {
    return (
      <PageTemplate title={t('detail.loading')} width="wide">
        <p role="status" className="text-sm text-muted-foreground">
          {t('section.loading')}
        </p>
      </PageTemplate>
    )
  }
  if (account.isError) {
    return (
      <PageTemplate title={t('detail.notFound')} width="wide">
        <p role="alert" className="text-sm text-destructive">
          {t('detail.loadFailed')}{' '}
          <Link to="/integrations" className="underline">
            {t('common.backToIntegrations')}
          </Link>
        </p>
      </PageTemplate>
    )
  }

  const a = account.data
  const isAwsRole = a.provider === 'aws' && a.method === 'role_assumption'
  const confirmName = deleteConfirmName(a)

  return (
    <PageTemplate
      icon={<CloudGlassIcon size="1.4em" />}
      logo={<CloudProviderMark provider={a.provider} className="size-8" />}
      title={a.label}
      description={t(`providers.${a.provider}`, { defaultValue: a.provider })}
      status={
        <StatusBadge
          state={accountStatusState(a.status)}
          label={t(`status.${a.status}`, { defaultValue: a.status })}
        />
      }
      breadcrumb={
        <Link
          to="/integrations"
          className="text-sm text-muted-foreground hover:underline"
        >
          {t('common.backToIntegrations')}
        </Link>
      }
      width="wide"
    >
      <Tabs defaultValue="overview">
        <TabsList aria-label={t('detail.tabs')}>
          <TabsTrigger value="overview">{t('detail.overview')}</TabsTrigger>
          <TabsTrigger value="access">{t('detail.access')}</TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="space-y-4 pt-2">
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <dt className="text-xs text-muted-foreground">
                {t('detail.method')}
              </dt>
              <dd>{a.method}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">
                {t('detail.lastVerified')}
              </dt>
              <dd>
                {a.last_verified_at
                  ? formatAbsoluteTime(a.last_verified_at)
                  : t('section.neverVerified')}
              </dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">
                {t('detail.defaultRegion')}
              </dt>
              <dd className="font-mono">{a.config.region ?? '-'}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">
                {t('detail.credential')}
              </dt>
              <dd>{a.linked ? t('detail.stored') : t('detail.missing')}</dd>
            </div>
          </dl>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              size="sm"
              disabled={!actions.verify || verify.isPending}
              onClick={() => {
                setError(null)
                verify.mutate(id, {
                  onSuccess: setVerifyResult,
                  onError: (e) => {
                    setError(messageOf(e))
                  },
                })
              }}
            >
              {t('detail.verify')}
            </Button>
            {isAwsRole ? (
              <Button
                size="sm"
                variant="outline"
                disabled={!actions.probe || probe.isPending}
                onClick={() => {
                  setError(null)
                  probe.mutate(
                    { id, capabilities: [...AWS_CAPABILITIES] },
                    {
                      onError: (e) => {
                        setError(messageOf(e))
                      },
                    },
                  )
                }}
              >
                {t('detail.probe')}
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="outline"
              className="ml-auto text-destructive"
              disabled={!actions.remove}
              onClick={() => {
                setConfirmOpen(true)
              }}
            >
              {t('detail.delete')}
            </Button>
          </div>
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          {verifyResult ? (
            <p
              role="status"
              className={
                verifyResult.ok
                  ? 'text-sm text-foreground'
                  : 'text-sm text-destructive'
              }
            >
              {verifyResult.message}
              {verifyResult.needs ? ` ${verifyResult.needs}` : ''}
            </p>
          ) : null}
          {probe.data ? (
            <CapabilityResults results={probe.data.capabilities} />
          ) : null}
          <section
            aria-labelledby="cloud-account-regions"
            className="space-y-1"
          >
            <h3
              id="cloud-account-regions"
              className="text-xs font-semibold text-muted-foreground"
            >
              {t('detail.regions')}
            </h3>
            {regions.isPending ? (
              <p className="text-sm text-muted-foreground">
                {t('section.loading')}
              </p>
            ) : regions.isError ? (
              <p className="text-sm text-muted-foreground">
                {t('detail.regionsFailed')}
              </p>
            ) : (
              <ul className="flex flex-wrap gap-1.5">
                {(regions.data ?? []).map((r) => (
                  <li
                    key={r.id}
                    className="rounded-md bg-muted px-2 py-0.5 font-mono text-xs"
                  >
                    {r.id}
                  </li>
                ))}
              </ul>
            )}
          </section>
        </TabsContent>
        <TabsContent value="access" className="pt-2">
          <CloudAccountAccess accountId={id} canShare={actions.share} />
        </TabsContent>
      </Tabs>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('detail.deleteTitle', { name: confirmName })}
        description={t('detail.deleteBody')}
        confirmLabel={t('detail.delete')}
        tone="destructive"
        requireTyped={confirmName}
        pending={remove.isPending}
        error={remove.isError ? messageOf(remove.error) : null}
        onConfirm={() => {
          remove.mutate(id, {
            onSuccess: () => {
              setConfirmOpen(false)
              void navigate({ to: '/integrations' })
            },
          })
        }}
      />
    </PageTemplate>
  )
}
