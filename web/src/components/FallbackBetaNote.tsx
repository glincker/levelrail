import { useTranslation } from 'react-i18next'
import { BetaBadge } from './BetaBadge'

/** FallbackBetaNote labels an sslip.io fallback address as a beta test address. */
export function FallbackBetaNote({ url }: { url?: string }) {
  const { t } = useTranslation('networkProxy')
  if (!url || !/\.sslip\.io(:\d+)?\/?$/i.test(url)) return null
  return (
    <p className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
      <BetaBadge hint={t('automaticDomain.betaHint')} />
      {t('automaticDomain.betaHint')}
    </p>
  )
}
