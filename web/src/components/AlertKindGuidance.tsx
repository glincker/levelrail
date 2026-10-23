import { useTranslation } from 'react-i18next'
import { ALERT_KIND_COPY, type AlertKindCopyKind } from './alertKindCopy'

export function AlertKindGuidance({
  kind,
  withDescription = true,
}: {
  kind: AlertKindCopyKind
  withDescription?: boolean
}) {
  const { t } = useTranslation('observability')
  const copy = ALERT_KIND_COPY[kind]
  return (
    <div className="space-y-1 text-sm text-muted-foreground">
      {withDescription ? <p>{t(copy.description)}</p> : null}
      <p>
        <span className="font-medium text-foreground">
          {t('alertKinds.whatToDo')}:
        </span>{' '}
        {t(copy.action)}
      </p>
    </div>
  )
}
