import { useTranslation } from 'react-i18next'
import { useNow } from '@/hooks/useNow'
import { ageOf } from '@/lib/trafficTime'

/** "12 s", "3 min" style label for an elapsed time, ticking every second. */
export function useAgeLabel(from?: string | Date | null): string | null {
  const { t } = useTranslation('traffic')
  const now = useNow(1000)
  if (!from) return null
  const age = ageOf(from, now)
  return age ? t(`age.${age.unit}`, { count: age.count }) : null
}
