import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { messageOf } from '../../lib/cloudAwsServices'
import {
  RDS_ENGINES,
  rdsCanStart,
  rdsCanStop,
  rdsClusterCanStart,
  rdsClusterCanStop,
  rdsNeedsTypedConfirm,
} from '../../lib/cloudRds'
import {
  useAwsRds,
  useAwsRdsAction,
  useAwsRdsClusterAction,
  useAwsRdsClusters,
  useAwsRdsClusterSnapshotDelete,
  useAwsRdsClusterSnapshots,
  useAwsRdsCreate,
  useAwsRdsEstimate,
  type AwsRdsAction,
} from '../../queries/cloudRds'
import { StateLine } from './AwsServiceShared'

function CreateDialog({
  id,
  onClose,
}: Readonly<{ id: string; onClose: () => void }>) {
  const { t } = useTranslation('cloudAccounts')
  const create = useAwsRdsCreate(id)
  const [name, setName] = useState('')
  const [engine, setEngine] = useState<string>(RDS_ENGINES[0])
  const [cls, setCls] = useState('')
  const [user, setUser] = useState('app')
  const [multiAz, setMultiAz] = useState(false)
  const estimate = useAwsRdsEstimate(id, engine, cls, multiAz)
  const typed = rdsNeedsTypedConfirm(engine, multiAz)
  const est = estimate.data
  return (
    <ConfirmDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
      title={t('services.rds.createTitle')}
      description={t('services.rds.createBody')}
      confirmLabel={t('services.rds.create')}
      requireTyped={typed ? name.trim() || undefined : undefined}
      confirmDisabled={
        name.trim() === '' || cls === '' || est?.class_allowed === false
      }
      pending={create.isPending}
      error={create.isError ? messageOf(create.error) : null}
      onConfirm={() => {
        create.mutate(
          {
            id: name.trim(),
            engine,
            class: cls.trim(),
            master_username: user.trim(),
            allocated_gb: 20,
            multi_az: multiAz,
            confirm: typed,
          },
          { onSuccess: onClose },
        )
      }}
    >
      <div className="space-y-2">
        <Input
          value={name}
          aria-label={t('services.rds.name')}
          placeholder={t('services.rds.name')}
          onChange={(e) => {
            setName(e.target.value)
          }}
        />
        <select
          className="h-9 w-full rounded-md border border-border bg-background px-2 text-sm"
          value={engine}
          aria-label={t('services.rds.engine')}
          onChange={(e) => {
            setEngine(e.target.value)
          }}
        >
          {RDS_ENGINES.map((e) => (
            <option key={e} value={e}>
              {e}
            </option>
          ))}
        </select>
        <Input
          value={cls}
          aria-label={t('services.rds.class')}
          onChange={(e) => {
            setCls(e.target.value)
          }}
        />
        <Input
          value={user}
          aria-label={t('services.rds.username')}
          onChange={(e) => {
            setUser(e.target.value)
          }}
        />
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={multiAz}
            onCheckedChange={(v) => {
              setMultiAz(v === true)
            }}
          />
          {t('services.rds.multiAz')}
        </label>
        <p className="text-xs text-muted-foreground">
          {est?.est_monthly_usd == null
            ? t('services.rds.noEstimate')
            : t('services.rds.estimate', {
                usd: est.est_monthly_usd.toFixed(2),
              })}
          {est?.class_allowed === false
            ? ` ${t('services.rds.classBlocked')}`
            : ''}
        </p>
        <p className="text-xs text-muted-foreground">
          {t('services.rds.passwordNote')}
        </p>
      </div>
    </ConfirmDialog>
  )
}

function ClusterSection({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const clusters = useAwsRdsClusters(id)
  const snaps = useAwsRdsClusterSnapshots(id)
  const act = useAwsRdsClusterAction(id)
  const del = useAwsRdsClusterSnapshotDelete(id)
  const list = clusters.data?.clusters ?? []
  const days = clusters.data?.max_stop_days ?? 0
  if (list.length === 0 && (snaps.data ?? []).length === 0) return null
  return (
    <div className="space-y-1">
      <h4 className="text-xs font-medium">{t('services.rds.clusters')}</h4>
      {act.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(act.error)}
        </p>
      ) : null}
      <ul className="space-y-1">
        {list.map((k) => (
          <li key={k.id} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{k.id}</span>
            <span className="text-xs text-muted-foreground">
              {k.status} · {k.engine}
              {k.status === 'stopped' && days > 0
                ? ` · ${t('services.rds.autoRestart', { days })}`
                : ''}
            </span>
            <span className="ml-auto flex gap-1">
              {k.managed ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    act.mutate({ cid: k.id, action: 'snapshot' })
                  }}
                >
                  {t('services.rds.snapshot')}
                </Button>
              ) : null}
              {rdsClusterCanStop(k) ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    act.mutate({ cid: k.id, action: 'stop' })
                  }}
                >
                  {t('services.rds.stop')}
                </Button>
              ) : null}
              {rdsClusterCanStart(k) ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    act.mutate({ cid: k.id, action: 'start' })
                  }}
                >
                  {t('services.rds.start')}
                </Button>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
      <ul className="space-y-1">
        {(snaps.data ?? []).map((s) => (
          <li key={s.id} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{s.id}</span>
            <span className="text-xs text-muted-foreground">
              {s.cluster_id} · {s.status}
            </span>
            {s.managed ? (
              <Button
                size="sm"
                variant="outline"
                className="ml-auto"
                onClick={() => {
                  del.mutate(s.id)
                }}
              >
                {t('services.rds.deleteSnapshot')}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  )
}

export function RdsCard({
  id,
  canCreate,
}: Readonly<{ id: string; canCreate: boolean }>) {
  const { t } = useTranslation('cloudAccounts')
  const list = useAwsRds(id)
  const act = useAwsRdsAction(id)
  const [creating, setCreating] = useState(false)
  const run = (rid: string, action: AwsRdsAction) => {
    act.mutate({ rid, action })
  }
  return (
    <div className="space-y-2">
      {canCreate ? (
        <Button
          size="sm"
          onClick={() => {
            setCreating(true)
          }}
        >
          {t('services.rds.create')}
        </Button>
      ) : null}
      {creating ? (
        <CreateDialog
          id={id}
          onClose={() => {
            setCreating(false)
          }}
        />
      ) : null}
      {act.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(act.error)}
        </p>
      ) : null}
      <StateLine
        pending={list.isPending}
        error={list.error}
        empty={list.data?.length === 0 ? t('services.rds.empty') : undefined}
      />
      <ul className="space-y-1">
        {(list.data ?? []).map((i) => (
          <li key={i.id} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="font-mono text-xs">{i.id}</span>
            <span className="text-xs text-muted-foreground">
              {i.status} · {i.engine} · {i.class}
              {i.est_monthly_usd == null
                ? ''
                : ` · ${t('services.rds.perMonth', { usd: i.est_monthly_usd.toFixed(2) })}`}
            </span>
            <span className="ml-auto flex gap-1">
              {i.managed ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    run(i.id, 'snapshot')
                  }}
                >
                  {t('services.rds.snapshot')}
                </Button>
              ) : (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    run(i.id, 'adopt')
                  }}
                >
                  {t('services.rds.adopt')}
                </Button>
              )}
              {rdsCanStop(i) ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    run(i.id, 'stop')
                  }}
                >
                  {t('services.rds.stop')}
                </Button>
              ) : null}
              {rdsCanStart(i) ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    run(i.id, 'start')
                  }}
                >
                  {t('services.rds.start')}
                </Button>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
      <ClusterSection id={id} />
    </div>
  )
}
