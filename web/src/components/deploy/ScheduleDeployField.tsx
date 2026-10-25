import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import type { ScheduleAtError } from '../../lib/scheduleAt'

export interface ScheduleDeployValue {
  enabled: boolean
  when: string
}

// The "Schedule for later" option of the deploy dialog: a checkbox that
// reveals a date and time picker with the viewer's timezone shown.
export function ScheduleDeployField({
  value,
  error,
  onChange,
}: {
  value: ScheduleDeployValue
  error: ScheduleAtError | null
  onChange: (next: ScheduleDeployValue) => void
}) {
  const { t } = useTranslation('smartDeploy')
  const id = useId()
  const errorId = `${id}-error`
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
  return (
    <div className="space-y-2">
      <label htmlFor={id} className="flex items-center gap-2 text-sm">
        <Checkbox
          id={id}
          checked={value.enabled}
          onCheckedChange={(checked) => {
            onChange({ ...value, enabled: checked === true })
          }}
        />
        {t('schedule.toggle')}
      </label>
      {value.enabled ? (
        <div className="space-y-1.5 pl-6">
          <label htmlFor={`${id}-when`} className="text-xs font-medium">
            {t('schedule.whenLabel')}
          </label>
          <Input
            id={`${id}-when`}
            type="datetime-local"
            value={value.when}
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? errorId : undefined}
            onChange={(e) => {
              onChange({ ...value, when: e.target.value })
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('schedule.zone', { zone })}
          </p>
          {error ? (
            <p id={errorId} role="alert" className="text-xs text-destructive">
              {t(`schedule.${error}`)}
            </p>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
