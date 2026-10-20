import { useTranslation } from 'react-i18next'
import { StatusPill, type Tone } from '@/components/kit'
import { Button } from '../ui/button'
import type { UpgradeCheck, UpgradeCheckStatus } from '../../queries/updates'
import {
  useBackupNow,
  useRecheckPreflight,
} from '../../queries/upgradeSchedule'

const CHECK_TONE: Record<UpgradeCheckStatus, Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
  unknown: 'neutral',
  pending: 'info',
  skipped: 'neutral',
}

export function UpgradeCheckRow({ check }: { check: UpgradeCheck }) {
  const { t } = useTranslation('updates')
  const backup = useBackupNow()
  const recheck = useRecheckPreflight()
  const open =
    check.status !== 'ok' &&
    check.status !== 'unknown' &&
    check.status !== 'skipped'
  const fix = check.fix
  const error = backup.error ?? recheck.error

  return (
    <li className="flex items-start gap-3 text-sm">
      <StatusPill
        tone={CHECK_TONE[check.status]}
        label={t(`preflight.status.${check.status}`)}
        size="sm"
      />
      <span className="min-w-0 flex-1 space-y-1">
        <span className="block font-medium text-foreground">{check.name}</span>
        <span className="block text-xs text-muted-foreground">
          {check.message}
        </span>
        {open && check.hint ? (
          <span className="block text-xs text-foreground">{check.hint}</span>
        ) : null}
        {open && check.protects ? (
          <span className="block text-xs text-muted-foreground">
            {t('preflight.protects', { what: check.protects })}
          </span>
        ) : null}
        {open && fix?.action === 'backup_now' ? (
          <span className="block pt-1">
            <Button
              type="button"
              size="sm"
              disabled={backup.isPending}
              onClick={() => backup.mutate()}
            >
              {backup.isPending
                ? t('preflight.fix.backingUp')
                : t('preflight.fix.backupNow')}
            </Button>
          </span>
        ) : null}
        {open && fix?.action === 'recheck' ? (
          <span className="block pt-1">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={recheck.isPending}
              onClick={() => recheck.mutate()}
            >
              {recheck.isPending
                ? t('preflight.fix.rechecking')
                : t('preflight.fix.recheck')}
            </Button>
          </span>
        ) : null}
        {error ? (
          <span role="alert" className="block text-xs text-destructive">
            {error.message}
          </span>
        ) : null}
      </span>
    </li>
  )
}
