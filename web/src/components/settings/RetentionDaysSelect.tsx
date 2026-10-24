import { useTranslation } from 'react-i18next'
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
  onChange,
}: {
  id?: string
  value: number
  allowed: number[]
  /** Positive windows shorter than this are shown but cannot be picked. */
  floor?: number
  disabled?: boolean
  ariaLabel: string
  onChange: (days: number) => void
}) {
  const { t } = useTranslation('runtimeSettings')
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
        {allowed.map((days) => (
          <SelectItem
            key={days}
            value={String(days)}
            disabled={belowFloor(days, floor)}
          >
            {label(days)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
