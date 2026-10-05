import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { usePITRStatus } from '../queries/pitr'
import {
  useDiscardMajorUpgradeSnapshot,
  useMajorUpgrades,
  useRollbackMajorUpgrade,
  useStartMajorUpgrade,
  type MajorUpgrade,
} from '../queries/majorUpgrades'
import type { DatabaseResource } from '../types/databaseDetail'

function HistoryRow({
  upgrade,
  name,
}: {
  upgrade: MajorUpgrade
  name: string
}) {
  const { t } = useTranslation('databases')
  const rollback = useRollbackMajorUpgrade(name)
  const discard = useDiscardMajorUpgradeSnapshot(name)
  const settled = upgrade.status !== 'running'
  const hasSnapshot = Boolean(upgrade.snapshot_volume) && settled
  const error = rollback.error ?? discard.error
  return (
    <li className="flex flex-col gap-1 border-b border-border py-2 last:border-b-0">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <span className="font-mono">
          {upgrade.from_version} &rarr; {upgrade.to_version}
        </span>
        <span>{t(`majorUpgrade.status.${upgrade.status}`)}</span>
        {upgrade.status === 'running' && upgrade.phase ? (
          <span className="text-muted-foreground">{upgrade.phase}</span>
        ) : null}
        {hasSnapshot ? (
          <span className="ml-auto flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="xs"
              disabled={rollback.isPending}
              onClick={() => {
                if (
                  window.confirm(
                    t('majorUpgrade.rollbackConfirm', {
                      version: upgrade.from_version,
                    }),
                  )
                ) {
                  rollback.mutate(upgrade.id, {
                    onSuccess: () =>
                      toast.add({
                        title: t('majorUpgrade.rollbackStarted'),
                        type: 'success',
                      }),
                  })
                }
              }}
            >
              {t('majorUpgrade.rollback')}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="xs"
              disabled={discard.isPending}
              onClick={() => {
                if (window.confirm(t('majorUpgrade.discardConfirm'))) {
                  discard.mutate(upgrade.id, {
                    onSuccess: () =>
                      toast.add({
                        title: t('majorUpgrade.discarded'),
                        type: 'success',
                      }),
                  })
                }
              }}
            >
              {t('majorUpgrade.discard')}
            </Button>
          </span>
        ) : null}
      </div>
      {upgrade.error ? (
        <p className="text-xs break-words text-destructive">{upgrade.error}</p>
      ) : null}
      {error ? (
        <p className="text-xs text-destructive" role="alert">
          {error.message}
        </p>
      ) : null}
    </li>
  )
}

// Postgres only: starts a guarded major upgrade and lists past attempts with
// rollback and snapshot cleanup.
export function MajorUpgradeCard({ database }: { database: DatabaseResource }) {
  const { t } = useTranslation('databases')
  const name = database.name
  const history = useMajorUpgrades(name)
  const start = useStartMajorUpgrade(name)
  const [version, setVersion] = useState('')
  const [confirm, setConfirm] = useState('')
  const running = history.data?.some((u) => u.status === 'running') ?? false
  const pitrOn = usePITRStatus(name).data?.enabled === true

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('majorUpgrade.title')}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <p className="text-sm text-muted-foreground">
          {t('majorUpgrade.description')}
        </p>
        {pitrOn ? (
          <p className="text-sm text-destructive">
            {t('majorUpgrade.pitrBlocked')}
          </p>
        ) : null}
        <div className="flex flex-wrap items-end gap-3">
          <label className="space-y-1 text-sm font-medium">
            {t('majorUpgrade.versionLabel')}
            <Input
              value={version}
              placeholder={t('majorUpgrade.versionPlaceholder')}
              onChange={(e) => {
                setVersion(e.target.value)
              }}
              className="w-28 font-mono"
            />
          </label>
          <label className="space-y-1 text-sm font-medium">
            {t('majorUpgrade.confirmLabel')}
            <Input
              value={confirm}
              onChange={(e) => {
                setConfirm(e.target.value)
              }}
              className="w-56 font-mono"
            />
          </label>
          <Button
            type="button"
            variant="destructive"
            disabled={
              start.isPending ||
              running ||
              pitrOn ||
              version.trim() === '' ||
              confirm !== name
            }
            onClick={() => {
              start.mutate(version.trim(), {
                onSuccess: () => {
                  setConfirm('')
                  toast.add({
                    title: t('majorUpgrade.started', { name }),
                    type: 'success',
                  })
                },
              })
            }}
          >
            {start.isPending
              ? t('majorUpgrade.starting')
              : t('majorUpgrade.start')}
          </Button>
        </div>
        {start.isError ? (
          <p className="text-sm text-destructive" role="alert">
            {start.error.message}
          </p>
        ) : null}
        <h3 className="text-sm font-medium">{t('majorUpgrade.history')}</h3>
        {history.data && history.data.length > 0 ? (
          <ul className="text-sm">
            {history.data.map((u) => (
              <HistoryRow key={u.id} upgrade={u} name={name} />
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground italic">
            {t('majorUpgrade.empty')}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
