import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { belowFloor } from '../../lib/cleanup'

const DAYS_PER_YEAR = 365

/** Preset picker for a retention window in days; 0 keeps data forever. */
export function RetentionDaysSelect({
  id,
  value,
  allowed,
  disabled,
  ariaLabel,
  floor = 0,
  lockedHint,
  onChange,
}: {
  id?: string
  value: number
  allowed: number[]
  /** Positive windows shorter than this are shown but cannot be picked. */
  floor?: number
  /** Describes why a window below the floor is locked; read by screen readers on the option. */
  lockedHint?: string
  disabled?: boolean
  ariaLabel: string
  onChange: (days: number) => void
}) {
  const { t } = useTranslation('runtimeSettings')
  const hintId = useId()
  const label = (days: number) => {
    if (days === 0) return t('retention.never')
    if (days === DAYS_PER_YEAR) return t('retention.year')
    return t('retention.days', { count: days })
  }
  return (
    <Select
      value={String(value)}
      disabled={disabled}
      onValueChange={(next) => {
        if (next !== null) onChange(Number(next))
      }}
    >
      <SelectTrigger id={id} size="sm" aria-label={ariaLabel}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {allowed.map((days) => {
          const locked = belowFloor(days, floor)
          return (
            <SelectItem
              key={days}
              value={String(days)}
              disabled={locked}
              aria-disabled={locked || undefined}
              aria-describedby={locked && lockedHint ? hintId : undefined}
            >
              {label(days)}
              {locked ? (
                <LockGlassIcon className="size-4" aria-hidden="true" />
              ) : null}
            </SelectItem>
          )
        })}
        {lockedHint ? (
          <span id={hintId} className="sr-only">
            {lockedHint}
          </span>
        ) : null}
      </SelectContent>
    </Select>
  )
}
