import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusPill, type Tone } from '@/components/kit'
import {
  preflightQueryOptions,
  type UpgradeCheckStatus,
} from '../../queries/updates'
import { CopyCommand } from './CopyCommand'
import { ReleaseNotes } from './ReleaseNotes'

const STATUS_TONE: Record<UpgradeCheckStatus, Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
  unknown: 'neutral',
}

export function UpgradePreflight() {
  const { t } = useTranslation('updates')
  const { data, isPending, isError } = useQuery(preflightQueryOptions())
  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">{t('preflight.running')}</p>
    )
  }
  if (isError) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('preflight.unavailable')}
      </p>
    )
  }
  return (
    <div className="space-y-4">
      {data.release_notes ? (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">
            {t('notes.title')}
          </p>
          <div className="max-h-48 overflow-auto rounded-md bg-muted px-3 py-2">
            <ReleaseNotes markdown={data.release_notes} />
          </div>
        </div>
      ) : null}
      <ul className="space-y-2" aria-label={t('preflight.checksLabel')}>
        {data.checks.map((c) => (
          <li key={c.code} className="flex items-start gap-3 text-sm">
            <StatusPill
              tone={STATUS_TONE[c.status]}
              label={t(`preflight.status.${c.status}`)}
              size="sm"
            />
            <span className="min-w-0 flex-1">
              <span className="font-medium text-foreground">{c.name}</span>
              <span className="block text-xs text-muted-foreground">
                {c.message}
              </span>
              {c.code === 'release_verifier' &&
              c.status === 'warn' &&
              data.cosign_command ? (
                <span className="mt-2 block">
                  <CopyCommand
                    label={t('preflight.cosignCommand')}
                    command={data.cosign_command}
                  />
                </span>
              ) : null}
            </span>
          </li>
        ))}
      </ul>
      {data.blocked ? (
        <p className="text-sm text-destructive">{t('preflight.blocked')}</p>
      ) : data.update_available ? (
        <div className="space-y-3">
          <CopyCommand
            label={t('preflight.upgradeCommand')}
            command={data.upgrade_command}
          />
          <CopyCommand
            label={t('preflight.rollbackCommand')}
            command={data.rollback_command}
          />
        </div>
      ) : null}
    </div>
  )
}
