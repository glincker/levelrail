import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { AutoscaleDay } from '../../queries/autoscale'
import { DAYS, newScheduleDraft, type ScheduleDraft } from './scheduleDrafts'

/** AutoscaleSchedules edits the time windows that raise the minimum replicas. */
export function AutoscaleSchedules({
  drafts,
  onChange,
}: {
  drafts: ScheduleDraft[]
  onChange: (next: ScheduleDraft[]) => void
}) {
  const { t } = useTranslation('autoscaleSchedule')
  const patch = (i: number, p: Partial<ScheduleDraft>) =>
    onChange(drafts.map((d, j) => (j === i ? { ...d, ...p } : d)))
  const toggle = (i: number, current: AutoscaleDay[], day: AutoscaleDay) =>
    patch(i, {
      days: current.includes(day)
        ? current.filter((x) => x !== day)
        : DAYS.filter((x) => x === day || current.includes(x)),
    })
  return (
    <fieldset className="space-y-3 rounded-lg border border-border p-3">
      <legend className="px-1 text-sm font-medium">{t('title')}</legend>
      <p className="text-xs text-muted-foreground">{t('help')}</p>
      {drafts.map((d, i) => (
        <div
          key={i}
          role="group"
          aria-label={t('window', { n: i + 1 })}
          className="space-y-3 rounded-md border border-border p-3"
        >
          <div className="flex flex-wrap gap-1.5">
            {DAYS.map((day) => (
              <label
                key={day}
                className="cursor-pointer rounded-md border border-border px-2 py-1 text-xs has-[:checked]:border-ring has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"
              >
                <input
                  type="checkbox"
                  className="sr-only"
                  checked={d.days.includes(day)}
                  onChange={() => toggle(i, d.days, day)}
                />
                {t(`day.${day}`)}
              </label>
            ))}
          </div>
          <div className="grid gap-3 sm:grid-cols-4">
            <div className="space-y-1.5">
              <Label htmlFor={`autoscale-sched-${i}-start`}>{t('start')}</Label>
              <Input
                id={`autoscale-sched-${i}-start`}
                type="time"
                value={d.start}
                onChange={(e) => patch(i, { start: e.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor={`autoscale-sched-${i}-end`}>{t('end')}</Label>
              <Input
                id={`autoscale-sched-${i}-end`}
                type="time"
                value={d.end}
                onChange={(e) => patch(i, { end: e.target.value })}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor={`autoscale-sched-${i}-tz`}>{t('timezone')}</Label>
              <Input
                id={`autoscale-sched-${i}-tz`}
                value={d.timezone}
                placeholder="UTC"
                onChange={(e) => patch(i, { timezone: e.target.value })}
                aria-describedby={`autoscale-sched-${i}-tz-help`}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor={`autoscale-sched-${i}-min`}>{t('min')}</Label>
              <Input
                id={`autoscale-sched-${i}-min`}
                inputMode="numeric"
                value={d.min}
                onChange={(e) => patch(i, { min: e.target.value })}
              />
            </div>
          </div>
          <p
            id={`autoscale-sched-${i}-tz-help`}
            className="text-xs text-muted-foreground"
          >
            {t('timezoneHelp')}
          </p>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => onChange(drafts.filter((_, j) => j !== i))}
          >
            {t('remove', { n: i + 1 })}
          </Button>
        </div>
      ))}
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => onChange([...drafts, newScheduleDraft()])}
      >
        {t('add')}
      </Button>
    </fieldset>
  )
}
