import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { messageOf } from '../../lib/cloudAwsServices'
import { snapshotTarget } from '../../lib/cloudAwsOps'
import {
  useAwsSnapshotCreate,
  useAwsSnapshotDelete,
  useAwsSnapshots,
  useAwsVolumes,
} from '../../queries/cloudAwsOps'
import { StateLine } from './AwsServiceShared'

export function SnapshotsCard({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('cloudAccounts')
  const snaps = useAwsSnapshots(id)
  const vols = useAwsVolumes(id)
  const create = useAwsSnapshotCreate(id)
  const del = useAwsSnapshotDelete(id)
  const [target, setTarget] = useState('')
  const [excludeBoot, setExcludeBoot] = useState(false)
  const err = create.error ?? del.error
  return (
    <div className="space-y-3">
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate(
            { ...snapshotTarget(target), exclude_boot: excludeBoot },
            {
              onSuccess: () => {
                setTarget('')
              },
            },
          )
        }}
      >
        <Input
          className="w-56"
          value={target}
          aria-label={t('services.ebs.target')}
          placeholder={t('services.ebs.target')}
          onChange={(e) => {
            setTarget(e.target.value)
          }}
        />
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={excludeBoot}
            onCheckedChange={(v) => {
              setExcludeBoot(v === true)
            }}
          />
          {t('services.ebs.excludeBoot')}
        </label>
        <Button
          size="sm"
          type="submit"
          disabled={target.trim() === '' || create.isPending}
        >
          {t('services.ebs.create')}
        </Button>
      </form>
      {err ? (
        <p role="alert" className="text-sm text-destructive">
          {messageOf(err)}
        </p>
      ) : null}
      <section className="space-y-1" aria-label={t('services.ebs.volumes')}>
        <h4 className="text-xs font-medium text-muted-foreground">
          {t('services.ebs.volumes')}
        </h4>
        <StateLine
          pending={vols.isPending}
          error={vols.error}
          empty={
            vols.data?.length === 0 ? t('services.ebs.emptyVolumes') : undefined
          }
        />
        <ul className="space-y-1">
          {(vols.data ?? []).map((v) => (
            <li
              key={v.id}
              className="flex flex-wrap items-center gap-2 text-sm"
            >
              <span className="font-mono text-xs">{v.id}</span>
              <span className="text-xs text-muted-foreground">
                {v.state} · {t('services.ebs.size', { gb: v.size_gb })} ·{' '}
                {v.type}
                {v.instance_id ? ` · ${v.instance_id}` : ''}
              </span>
            </li>
          ))}
        </ul>
      </section>
      <section className="space-y-1" aria-label={t('services.ebs.snapshots')}>
        <h4 className="text-xs font-medium text-muted-foreground">
          {t('services.ebs.snapshots')}
        </h4>
        <StateLine
          pending={snaps.isPending}
          error={snaps.error}
          empty={
            snaps.data?.length === 0
              ? t('services.ebs.emptySnapshots')
              : undefined
          }
        />
        <ul className="space-y-1">
          {(snaps.data ?? []).map((s) => (
            <li
              key={s.id}
              className="flex flex-wrap items-center gap-2 text-sm"
            >
              <span className="font-mono text-xs">{s.id}</span>
              <span className="text-xs text-muted-foreground">
                {s.state} · {s.volume_id} ·{' '}
                {t('services.ebs.size', { gb: s.size_gb })}
                {s.expires_at
                  ? ` · ${t('services.ebs.expires', { when: s.expires_at })}`
                  : ''}
              </span>
              {s.managed ? (
                <Button
                  size="sm"
                  variant="outline"
                  className="ml-auto"
                  disabled={del.isPending}
                  onClick={() => {
                    del.mutate(s.id)
                  }}
                >
                  {t('services.ebs.delete')}
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      </section>
    </div>
  )
}
