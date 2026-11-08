import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBulkMuteAlertRules } from '../queries/alerts'
import type { AlertRule } from '../types/alerts'

export type RuleFilter = 'all' | 'firing' | 'pending' | 'ok' | 'silenced'

const FILTERS: RuleFilter[] = ['all', 'firing', 'pending', 'ok', 'silenced']
const DURATIONS = ['1h', '4h', '24h', '168h'] as const

// Filter chips plus "mute the shown rules for N" so a noisy deploy window
// is one click, not one dialog per rule.
export function AlertRulesBulkBar({
  appName,
  filter,
  onFilter,
  shown,
}: {
  appName: string
  filter: RuleFilter
  onFilter: (f: RuleFilter) => void
  shown: AlertRule[]
}) {
  const { t } = useTranslation('alertSetup')
  const [duration, setDuration] = useState<string>(DURATIONS[1])
  const mute = useBulkMuteAlertRules(appName)
  const targets = shown.filter((r) => !r.silenced)

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div
        role="group"
        aria-label={t('rules.bulk.filter')}
        className="flex flex-wrap gap-1"
      >
        {FILTERS.map((f) => (
          <Button
            key={f}
            type="button"
            size="xs"
            variant={filter === f ? 'secondary' : 'ghost'}
            aria-pressed={filter === f}
            onClick={() => onFilter(f)}
          >
            {t(`rules.bulk.filters.${f}`)}
          </Button>
        ))}
      </div>
      <label className="ml-auto flex items-center gap-2 text-xs">
        {t('rules.bulk.duration')}
        <select
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
          className="rounded-md border bg-background px-2 py-1 text-xs"
        >
          {DURATIONS.map((d) => (
            <option key={d} value={d}>
              {t(`rules.bulk.durations.${d}`)}
            </option>
          ))}
        </select>
      </label>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={mute.isPending || targets.length === 0}
        onClick={() =>
          mute.mutate(
            {
              ruleIds: targets.map((r) => r.id),
              duration,
              reason: t('rules.bulk.reason'),
            },
            {
              onSuccess: (items) => {
                const failed = items.filter((i) => i.error).length
                toast.add({
                  title: t('rules.bulk.done', { count: items.length - failed }),
                  description:
                    failed > 0 ? t('rules.bulk.failed', { count: failed }) : '',
                  type: failed > 0 ? 'error' : 'success',
                })
              },
              onError: (e) =>
                toast.add({
                  title: t('rules.bulk.error'),
                  description: e.message,
                  type: 'error',
                }),
            },
          )
        }
      >
        {t('rules.bulk.mute', { count: targets.length })}
      </Button>
    </div>
  )
}
