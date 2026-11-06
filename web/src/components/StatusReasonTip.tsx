import { useTranslation } from 'react-i18next'
import { useStatusReasons } from '../queries/statusReasons'
import { InfoTip } from './kit'

interface StatusReasonTipProps {
  code: string | undefined
}

export function StatusReasonTip({ code }: StatusReasonTipProps) {
  const { t } = useTranslation('common')
  const { data } = useStatusReasons()
  const reason = code ? data?.find((r) => r.code === code) : undefined
  if (!reason) return null
  return (
    <InfoTip label={t('statusReasons.about')}>
      <p>{reason.meaning}</p>
      <p className="mt-1.5">
        <span className="font-medium text-foreground">
          {t('statusReasons.next')}
        </span>{' '}
        {reason.next_step}
      </p>
    </InfoTip>
  )
}
