import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import type { Tone } from '../kit/tone'
import type { AbilityRisk } from '../../queries/iamBuilder'

const RISK_TONE: Record<AbilityRisk, Tone> = {
  read: 'info',
  write: 'warning',
  sensitive: 'warning',
  root: 'danger',
}

export function RiskBadge({ risk }: { risk: AbilityRisk }) {
  const { t } = useTranslation('iam')
  const tone = TONE[RISK_TONE[risk]]
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center rounded-full border px-2 py-0.5 text-xs',
        tone.text,
        tone.soft,
        tone.border,
      )}
    >
      {t(`risk.${risk}`)}
    </span>
  )
}
