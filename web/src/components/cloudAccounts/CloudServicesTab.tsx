import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useIsRoot } from '../../hooks/useIsRoot'
import { AWS_SERVICE_SETTING, messageOf } from '../../lib/cloudAwsServices'
import { formatAbsoluteTime } from '../../lib/relativeTime'
import {
  cloudAwsKeys,
  useAwsCleanupPlan,
  useAwsGuard,
  useSetAwsKillSwitch,
} from '../../queries/cloudAwsServices'
import { useSetRuntimeSetting } from '../../queries/runtimeSettings'
import type { AwsServiceName } from '../../types/cloudAccounts'
import { AcmBody, AuditBody, EcrBody, SecretsBody } from './AwsServiceCards'
import { ServiceShell, StateLine } from './AwsServiceShared'
import { IdleCard } from './IdleCard'
import { MetricsCard } from './MetricsCard'
import { ElastiCacheCard } from './ElastiCacheCard'
import { RdsCard } from './RdsCard'
import { SnapshotsCard } from './SnapshotsCard'

const SERVICES: AwsServiceName[] = [
  'ecr',
  'acm',
  'secrets',
  'rds',
  'elasticache',
  'ebs',
  'cloudwatch',
  'cloudtrail',
]

function GuardPanel({
  accountId,
  isRoot,
}: Readonly<{ accountId: string; isRoot: boolean }>) {
  const { t } = useTranslation('cloudAccounts')
  const guard = useAwsGuard(accountId)
  const kill = useSetAwsKillSwitch(accountId)
  const [planOn, setPlanOn] = useState(false)
  const plan = useAwsCleanupPlan(accountId, planOn)
  const g = guard.data
  const orphans = [
    ...(plan.data?.orphans.volumes ?? []).map((o) => ({
      ...o,
      kind: 'volume',
    })),
    ...(plan.data?.orphans.addresses ?? []).map((o) => ({
      ...o,
      kind: 'address',
    })),
    ...(plan.data?.orphans.snapshots ?? []).map((o) => ({
      ...o,
      kind: 'snapshot',
    })),
  ]
  return (
    <section
      aria-labelledby="aws-guard"
      className="space-y-2 rounded-lg border border-border p-3"
    >
      <h3 id="aws-guard" className="text-sm font-semibold">
        {t('services.guard.title')}
      </h3>
      <StateLine pending={guard.isPending} error={guard.error} />
      {g ? (
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
          <span>
            {t('services.guard.cap')}:{' '}
            <span className="font-mono">
              {g.monthly_cap_usd > 0
                ? `$${g.monthly_cap_usd}`
                : t('services.guard.noCap')}
            </span>
          </span>
          <span>
            {t('services.guard.mtd')}:{' '}
            <span className="font-mono">
              {g.mtd_spend_usd === null
                ? t('services.guard.unavailable')
                : `$${g.mtd_spend_usd.toFixed(2)}`}
            </span>
          </span>
          <label className="flex items-center gap-2">
            {t('services.guard.killSwitch')}
            <Switch
              size="sm"
              checked={g.kill_switch}
              disabled={!isRoot || kill.isPending}
              aria-label={t('services.guard.killSwitch')}
              onCheckedChange={(v) => {
                kill.mutate(v)
              }}
            />
          </label>
          {isRoot ? null : (
            <span className="text-xs text-muted-foreground">
              {t('services.guard.rootOnly')}
            </span>
          )}
        </div>
      ) : null}
      {kill.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(kill.error)}
        </p>
      ) : null}
      <div className="flex items-center gap-2">
        <h4 className="text-xs font-semibold text-muted-foreground">
          {t('services.guard.cleanup')}
        </h4>
        <span className="rounded-md bg-muted px-1.5 py-0.5 text-xs">
          {t('services.guard.dryRun')}
        </span>
        <Button
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={plan.isFetching}
          onClick={() => {
            if (planOn) void plan.refetch()
            else setPlanOn(true)
          }}
        >
          {t('services.guard.loadPlan')}
        </Button>
      </div>
      {planOn ? (
        <StateLine pending={plan.isPending} error={plan.error} />
      ) : null}
      {plan.data ? (
        plan.data.expired?.length || orphans.length ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('services.guard.resource')}</TableHead>
                <TableHead>{t('services.guard.reason')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(plan.data.expired ?? []).map((x) => (
                <TableRow key={x.arn}>
                  <TableCell className="font-mono text-xs">{x.arn}</TableCell>
                  <TableCell className="text-xs">
                    {t('services.guard.expired', {
                      when: formatAbsoluteTime(x.expired_at),
                    })}
                  </TableCell>
                </TableRow>
              ))}
              {orphans.map((o) => (
                <TableRow key={`${o.kind}-${o.id}`}>
                  <TableCell className="font-mono text-xs">{o.id}</TableCell>
                  <TableCell className="text-xs">
                    {t('services.guard.orphan', { kind: o.kind })} {o.detail}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('services.guard.nothing')}
          </p>
        )
      ) : null}
    </section>
  )
}

export function CloudServicesTab({
  accountId,
}: Readonly<{ accountId: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const qc = useQueryClient()
  const isRoot = useIsRoot()
  const guard = useAwsGuard(accountId)
  const setting = useSetRuntimeSetting()
  const [error, setError] = useState<string | null>(null)
  const enabledOf = (s: AwsServiceName) => guard.data?.services?.[s] === true

  const toggle = (s: AwsServiceName, on: boolean) => {
    setError(null)
    setting.mutate(
      { key: AWS_SERVICE_SETTING[s], value: on },
      {
        onSuccess: () => {
          void qc.invalidateQueries({ queryKey: cloudAwsKeys.all(accountId) })
        },
        onError: (e) => {
          setError(messageOf(e))
        },
      },
    )
  }
  const shell = (s: AwsServiceName, body: ReactNode) => (
    <ServiceShell
      key={s}
      id={`aws-service-${s}`}
      title={t(`services.${s}.title`)}
      enabled={enabledOf(s)}
      canToggle={isRoot && guard.isSuccess}
      toggling={setting.isPending}
      onToggle={(on) => {
        toggle(s, on)
      }}
    >
      {body}
    </ServiceShell>
  )
  return (
    <div className="space-y-3">
      {error ? (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      ) : null}
      <GuardPanel accountId={accountId} isRoot={isRoot} />
      <div className="grid gap-3 lg:grid-cols-2">
        {SERVICES.map((s) =>
          shell(
            s,
            s === 'ecr' ? (
              <EcrBody id={accountId} />
            ) : s === 'acm' ? (
              <AcmBody id={accountId} />
            ) : s === 'secrets' ? (
              <SecretsBody id={accountId} />
            ) : s === 'rds' ? (
              <RdsCard id={accountId} canCreate={isRoot} />
            ) : s === 'elasticache' ? (
              <ElastiCacheCard id={accountId} canDelete={isRoot} />
            ) : s === 'ebs' ? (
              <SnapshotsCard id={accountId} />
            ) : s === 'cloudwatch' ? (
              <div className="space-y-4">
                <MetricsCard id={accountId} />
                <IdleCard id={accountId} />
              </div>
            ) : (
              <AuditBody id={accountId} />
            ),
          ),
        )}
      </div>
    </div>
  )
}
