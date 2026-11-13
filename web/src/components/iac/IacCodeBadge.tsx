import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'

const ERRORS_DOC = 'https://docs.levelrail.com/iac/errors'

// The error reference lists every code in one table, so there is no per-code anchor.
export function IacCodeBadge({ code }: Readonly<{ code: string }>) {
  const { t } = useTranslation('settings')
  return (
    <a
      href={ERRORS_DOC}
      target="_blank"
      rel="noreferrer"
      aria-label={t('iacStudio.codeDoc', { code })}
      className="rounded-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      <Badge variant="destructive" className="font-mono">
        {code}
      </Badge>
    </a>
  )
}

export function IacBetaBadge() {
  const { t } = useTranslation('settings')
  return <Badge variant="warning">{t('iacStudio.beta')}</Badge>
}
