import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ArrowUpGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { selfUpgradeAttemptsQueryOptions } from '../../queries/selfUpgrade'
import { AttemptRow } from './updates/AttemptRow'
import { AttemptsSummary } from './updates/AttemptsSummary'

const OLDER_PAGE = 10

export function UpgradeAttempts({
  currentVersion,
}: {
  currentVersion?: string
}) {
  const { t } = useTranslation('updates')
  const attempts = useQuery(selfUpgradeAttemptsQueryOptions(false))
  const [shown, setShown] = useState(0)
  const list = [...(attempts.data ?? [])].sort(
    (a, b) => Date.parse(b.started_at) - Date.parse(a.started_at),
  )
  const latest = list[0]
  if (!latest) {
    return (
      <div className="flex flex-col items-center gap-1 rounded-md border border-dashed border-border px-4 py-6 text-center">
        <ArrowUpGlassIcon
          className="size-7 text-muted-foreground"
          aria-hidden="true"
        />
        <p className="text-sm font-medium">{t('selfUpgrade.noAttempts')}</p>
        <p className="max-w-prose text-xs text-muted-foreground">
          {t('selfUpgrade.noAttemptsHint')}
        </p>
      </div>
    )
  }
  const older = list.slice(1)
  const visible = older.slice(0, shown)
  const remaining = older.length - visible.length
  return (
    <div>
      <AttemptsSummary attempts={list} currentVersion={currentVersion} />
      <ul className="space-y-2" aria-label={t('selfUpgrade.attemptsTitle')}>
        <AttemptRow key={latest.id} attempt={latest} defaultOpen />
        {visible.map((a) => (
          <AttemptRow key={a.id} attempt={a} />
        ))}
      </ul>
      {remaining > 0 ? (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="mt-2"
          onClick={() => setShown((n) => n + OLDER_PAGE)}
        >
          {t('selfUpgrade.showOlder', {
            count: Math.min(remaining, OLDER_PAGE),
          })}
        </Button>
      ) : null}
    </div>
  )
}
